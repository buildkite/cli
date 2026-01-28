package pipelinerun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
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
	config *AgentConfig
	cmd    *exec.Cmd
	cancel context.CancelFunc
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

	// Build command arguments
	args := []string{"start"}

	// Spawn multiple workers
	if a.config.Spawn > 1 {
		args = append(args, fmt.Sprintf("--spawn=%d", a.config.Spawn))
	}

	// Set endpoint
	if a.config.Endpoint != "" {
		args = append(args, fmt.Sprintf("--endpoint=%s", a.config.Endpoint))
	}

	// Set token
	token := a.config.Token
	if token == "" {
		token = "local-token"
	}
	args = append(args, fmt.Sprintf("--token=%s", token))

	// Set build path
	if a.config.BuildPath != "" {
		args = append(args, fmt.Sprintf("--build-path=%s", a.config.BuildPath))
	}

	// Set hooks path
	if a.config.HooksPath != "" {
		args = append(args, fmt.Sprintf("--hooks-path=%s", a.config.HooksPath))
	}

	// Set plugins path
	if a.config.PluginsPath != "" {
		args = append(args, fmt.Sprintf("--plugins-path=%s", a.config.PluginsPath))
	}

	// Add tags
	for _, tag := range a.config.Tags {
		args = append(args, fmt.Sprintf("--tags=%s", tag))
	}

	// Add experiments
	for _, exp := range a.config.Experiments {
		args = append(args, fmt.Sprintf("--experiment=%s", exp))
	}

	// Disable git mirrors for local runs
	args = append(args, "--no-git-mirrors")

	// Disable automatic ssh fingerprint verification
	args = append(args, "--no-ssh-keyscan")

	// Don't verify the server certificate (we're using a local mock)
	args = append(args, "--no-http2")

	if a.config.Debug {
		args = append(args, "--debug")
	}

	a.cmd = exec.CommandContext(ctx, a.config.BinaryPath, args...)

	// Set up environment
	a.cmd.Env = append(os.Environ(),
		"BUILDKITE_AGENT_ENDPOINT="+a.config.Endpoint,
		"BUILDKITE_AGENT_ACCESS_TOKEN="+token,
		"BUILDKITE_AGENT_DEBUG=true",
	)

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

	if a.cmd == nil || a.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM to process group
	pgid, err := syscall.Getpgid(a.cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
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
