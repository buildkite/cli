package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildkite/cli/v3/internal/pipelinerun"
)

// RunCmd is the command for running a pipeline locally
type RunCmd struct {
	File     string            `help:"Path to the pipeline file" short:"f" type:"path"`
	Step     []string          `help:"Run only specific steps (by ID or key, can repeat)" short:"s"`
	Spawn    int               `help:"Number of agent workers to spawn (0=auto)" default:"0"`
	Env      map[string]string `help:"Environment variables to set" short:"e"`
	DryRun   bool              `help:"Plan the pipeline without executing" name:"dry-run"`
	Text     bool              `help:"Output as text instead of JSON (for --dry-run)" name:"text"`
	NoAgent  bool              `help:"Run commands directly without buildkite-agent (disables plugins)" name:"no-agent"`
	AgentBin string            `help:"Path to buildkite-agent binary" name:"agent-bin" type:"path"`
	Port     int               `help:"Port for mock server (0=auto)" default:"0"`
	BuildDir string            `help:"Directory for build artifacts" name:"build-dir" type:"path"`
	Verbose  bool              `help:"Enable verbose output" short:"v"`
}

// Run executes the pipeline run command
func (cmd *RunCmd) Run() error {
	// Find pipeline file
	pipelineFile := cmd.File
	if pipelineFile == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}

		pipelineFile, err = pipelinerun.FindPipelineFile(cwd)
		if err != nil {
			return fmt.Errorf("no pipeline file specified and none found: %w", err)
		}
	}

	// Make path absolute
	if !filepath.IsAbs(pipelineFile) {
		cwd, _ := os.Getwd()
		pipelineFile = filepath.Join(cwd, pipelineFile)
	}

	// Build environment
	env := make(map[string]string)

	// Copy provided env
	for k, v := range cmd.Env {
		env[k] = v
	}

	// Set some defaults
	if _, ok := env["BUILDKITE_PIPELINE_SLUG"]; !ok {
		// Extract from pipeline file path
		dir := filepath.Dir(pipelineFile)
		env["BUILDKITE_PIPELINE_SLUG"] = filepath.Base(filepath.Dir(dir))
	}
	if _, ok := env["BUILDKITE_ORGANIZATION_SLUG"]; !ok {
		env["BUILDKITE_ORGANIZATION_SLUG"] = "local"
	}
	if _, ok := env["BUILDKITE_BRANCH"]; !ok {
		env["BUILDKITE_BRANCH"] = getCurrentBranch()
	}
	if _, ok := env["BUILDKITE_COMMIT"]; !ok {
		env["BUILDKITE_COMMIT"] = getCurrentCommit()
	}

	// Use agent mode by default (supports plugins), unless --no-agent is specified
	useAgent := !cmd.NoAgent

	config := &pipelinerun.RunConfig{
		PipelineFile: pipelineFile,
		Spawn:        cmd.Spawn,
		Env:          env,
		Port:         cmd.Port,
		UseAgent:     useAgent,
		AgentBinary:  cmd.AgentBin,
		BuildPath:    cmd.BuildDir,
		DryRun:       cmd.DryRun,
		Steps:        cmd.Step,
		JSON:         !cmd.Text, // Default to JSON, use --text for text output
		Debug:        cmd.Verbose,
		Output:       os.Stdout,
	}

	result, err := pipelinerun.Run(context.Background(), config)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("pipeline failed with %d failed jobs", result.FailedJobs)
	}

	return nil
}

// getCurrentBranch tries to get the current git branch
func getCurrentBranch() string {
	// Try to read from git
	data, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return "main"
	}

	head := strings.TrimSpace(string(data))
	if strings.HasPrefix(head, "ref: refs/heads/") {
		return strings.TrimPrefix(head, "ref: refs/heads/")
	}

	return "main"
}

// getCurrentCommit tries to get the current git commit
func getCurrentCommit() string {
	// Try to read from git
	data, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return "HEAD"
	}

	head := strings.TrimSpace(string(data))
	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimPrefix(head, "ref: ")
		commitData, err := os.ReadFile(filepath.Join(".git", ref))
		if err != nil {
			return "HEAD"
		}
		return strings.TrimSpace(string(commitData))
	}

	// Detached HEAD - HEAD contains the commit
	return head
}
