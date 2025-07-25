package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/buildkite/cli/v3/pkg/factory"
)

// Init command
type InitCmd struct{}

func (i *InitCmd) Help() string {
	return `Creates a basic .buildkite/pipeline.yml file to get started with Buildkite.

The interactive process will:
  - Prompt for a pipeline name
  - Prompt for a command to run (e.g., "npm test" or "make build")
  - Create .buildkite/pipeline.yml with a single build step

EXAMPLES:
  # Interactive setup
  bk init

  # Then create the pipeline on Buildkite
  bk pipeline create --name "My Pipeline"

For full pipeline.yml documentation, see:
https://buildkite.com/docs/pipelines/configure`
}

func (i *InitCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Check if we're in a git repository
	if f.GitRepository == nil {
		return fmt.Errorf("not in a git repository")
	}

	// Check if pipeline.yml already exists
	if _, err := os.Stat(".buildkite/pipeline.yml"); err == nil {
		return fmt.Errorf("pipeline.yml already exists")
	}

	fmt.Println("Creating pipeline.yml interactively...")

	// Get basic pipeline info
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Pipeline name: ")
	name, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)

	fmt.Print("Command to run (e.g., 'echo hello'): ")
	command, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	command = strings.TrimSpace(command)

	// Create basic pipeline content
	pipelineContent := fmt.Sprintf(`steps:
  - label: "Build"
    command: %s
`, command)

	// Create .buildkite directory if it doesn't exist
	if err := os.MkdirAll(".buildkite", 0755); err != nil {
		return fmt.Errorf("error creating .buildkite directory: %w", err)
	}

	// Write pipeline.yml
	if err := os.WriteFile(".buildkite/pipeline.yml", []byte(pipelineContent), 0644); err != nil {
		return fmt.Errorf("error writing pipeline.yml: %w", err)
	}

	fmt.Println("✓ Created .buildkite/pipeline.yml")
	fmt.Printf("Pipeline ready! You can create it in Buildkite with: bk pipeline create --name \"%s\"\n", name)

	return nil
}
