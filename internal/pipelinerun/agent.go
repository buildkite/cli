package pipelinerun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"text/template"
)

// AgentConfig holds configuration for the local agent
type AgentConfig struct {
	// Path to the buildkite-agent binary
	BinaryPath string

	// Number of parallel workers to spawn
	Spawn int

	// Endpoint URL for the mock server
	Endpoint string

	// Token for authentication
	Token string

	// Build directory for job execution
	BuildPath string

	// Hooks path
	HooksPath string

	// Plugins path
	PluginsPath string

	// Debug mode
	Debug bool

	// Tags for the agent
	Tags []string

	// Experiment flags
	Experiments []string
}

// Agent wraps the buildkite-agent process
type Agent struct {
	config        *AgentConfig
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	tempDir       string
	bootstrapPath string
}

// NewAgent creates a new agent wrapper
func NewAgent(config *AgentConfig) *Agent {
	return &Agent{
		config: config,
	}
}

// FindAgentBinary searches for the buildkite-agent binary
func FindAgentBinary() (string, error) {
	// Check if BUILDKITE_AGENT_PATH is set
	if path := os.Getenv("BUILDKITE_AGENT_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Check PATH
	path, err := exec.LookPath("buildkite-agent")
	if err == nil {
		return path, nil
	}

	// Check common installation locations
	candidates := []string{
		"/usr/local/bin/buildkite-agent",
		"/opt/homebrew/bin/buildkite-agent",
		filepath.Join(os.Getenv("HOME"), ".buildkite-agent", "bin", "buildkite-agent"),
	}

	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\buildkite-agent\buildkite-agent.exe`,
			`C:\buildkite-agent\buildkite-agent.exe`,
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("buildkite-agent not found in PATH or common locations")
}

// Start starts the buildkite-agent process
func (a *Agent) Start(ctx context.Context) error {
	ctx, a.cancel = context.WithCancel(ctx)

	// Create temp directory for bootstrap script and other files
	var err error
	a.tempDir, err = os.MkdirTemp("", "bk-agent-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	// Create bootstrap script
	if err := a.createBootstrapScript(); err != nil {
		return fmt.Errorf("creating bootstrap script: %w", err)
	}

	// Set up build and plugins paths
	buildPath := a.config.BuildPath
	if buildPath == "" {
		buildPath = filepath.Join(a.tempDir, "builds")
		if err := os.MkdirAll(buildPath, 0755); err != nil {
			return fmt.Errorf("creating build path: %w", err)
		}
	}

	pluginsPath := a.config.PluginsPath
	if pluginsPath == "" {
		pluginsPath = filepath.Join(a.tempDir, "plugins")
		if err := os.MkdirAll(pluginsPath, 0755); err != nil {
			return fmt.Errorf("creating plugins path: %w", err)
		}
	}

	// Build command arguments
	args := []string{"start"}

	// Spawn multiple workers
	if a.config.Spawn > 1 {
		args = append(args, fmt.Sprintf("--spawn=%d", a.config.Spawn))
	}

	// Disconnect after idle (exit when no jobs)
	args = append(args, "--disconnect-after-idle-timeout=10")

	// No color in output
	args = append(args, "--no-color")

	if a.config.Debug {
		args = append(args, "--debug")
	}

	a.cmd = exec.CommandContext(ctx, a.config.BinaryPath, args...)

	// Set token
	token := a.config.Token
	if token == "" {
		token = "local-token"
	}

	// Set up environment - this is how the agent finds our mock server
	a.cmd.Env = append(os.Environ(),
		"BUILDKITE_AGENT_ENDPOINT="+a.config.Endpoint,
		"BUILDKITE_AGENT_TOKEN="+token,
		"BUILDKITE_BUILD_PATH="+buildPath,
		"BUILDKITE_PLUGINS_PATH="+pluginsPath,
		"BUILDKITE_BOOTSTRAP_SCRIPT_PATH="+a.bootstrapPath,
		"BUILDKITE_SHELL=/bin/bash -e -c",
		"BUILDKITE_NO_LOCAL_HOOKS=false",
		"BUILDKITE_AGENT_NAME=local-agent",
	)

	if a.config.Debug {
		a.cmd.Env = append(a.cmd.Env, "BUILDKITE_AGENT_DEBUG=true")
	}

	// Connect stdout/stderr
	a.cmd.Stdout = os.Stdout
	a.cmd.Stderr = os.Stderr

	// Start in a new process group so we can kill all children
	a.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	return nil
}

// bootstrapTemplate is the script that runs jobs
const bootstrapTemplate = `#!/bin/bash
set -e

# Run the buildkite-agent bootstrap with plugin and command phases only
# We skip checkout since we're running locally
exec "{{.AgentBinary}}" bootstrap \
    --phases "plugin,command" \
    "$@"
`

func (a *Agent) createBootstrapScript() error {
	a.bootstrapPath = filepath.Join(a.tempDir, "bootstrap.sh")

	tmpl, err := template.New("bootstrap").Parse(bootstrapTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.OpenFile(a.bootstrapPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("creating bootstrap script: %w", err)
	}
	defer f.Close()

	data := struct {
		AgentBinary string
	}{
		AgentBinary: a.config.BinaryPath,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

// Wait waits for the agent to exit
func (a *Agent) Wait() error {
	if a.cmd == nil {
		return nil
	}
	return a.cmd.Wait()
}

// Stop stops the agent process
func (a *Agent) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}

	if a.cmd != nil && a.cmd.Process != nil {
		// Send SIGTERM to process group
		pgid, err := syscall.Getpgid(a.cmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		}
	}

	// Clean up temp directory
	if a.tempDir != "" {
		_ = os.RemoveAll(a.tempDir)
	}

	return nil
}

// Pid returns the process ID of the agent
func (a *Agent) Pid() int {
	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Pid
	}
	return 0
}
