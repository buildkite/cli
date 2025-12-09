package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

const (
	maxPipelineLimit = 3000
	pageSize         = 100
)

type ListCmd struct {
	Name       string `help:"Filter pipelines by name (supports partial matches, case insensitive)" short:"n"`
	Repository string `help:"Filter pipelines by repository URL (supports partial matches, case insensitive)" short:"r"`
	Limit      int    `help:"Maximum number of pipelines to return (max: 3000)" short:"l" default:"100"`
	Output     string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ListCmd) Help() string {
	return `List pipelines with optional filtering.

This command lists all pipelines in the current organization that match
the specified criteria. You can filter by pipeline name or repository URL.

Examples:
  # List all pipelines (default limit: 100)
  $ bk pipeline list

  # List pipelines matching a name pattern
  $ bk pipeline list --name pipeline

  # List pipelines by repository
  $ bk pipeline list --repo my-repo

  # Get more pipelines (automatically paginates)
  $ bk pipeline list --limit 500

  # Output as JSON
  $ bk pipeline list --name pipeline -o json

  # Use with other commands (e.g., get longest builds from matching pipelines)
  $ bk pipeline list --name pipeline | xargs -I {} bk build list --pipeline {} --since 48h --duration 1h
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	if c.Limit > maxPipelineLimit {
		return fmt.Errorf("limit cannot exceed %d pipelines (requested: %d)", maxPipelineLimit, c.Limit)
	}

	ctx := context.Background()
	return c.runPipelineList(ctx, f)
}

func (c *ListCmd) runPipelineList(ctx context.Context, f *factory.Factory) error {
	org := f.Config.OrganizationSlug()
	if org == "" {
		return fmt.Errorf("no organization configured. Use 'bk configure' to set up your organization")
	}

	listOpts := c.pipelineListOptionsFromFlags()

	var pipelines []buildkite.Pipeline
	var err error

	err = bk_io.SpinWhile(f, "Loading pipelines", func() {
		pipelines, err = c.fetchPipelines(ctx, f, org, listOpts)
	})
	if err != nil {
		return fmt.Errorf("failed to list pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		fmt.Fprintln(os.Stdout, "No pipelines found matching the specified criteria.")
		return nil
	}

	return c.displayPipelines(pipelines)
}

func (c *ListCmd) pipelineListOptionsFromFlags() *buildkite.PipelineListOptions {
	listOpts := &buildkite.PipelineListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: pageSize,
		},
	}

	if c.Name != "" {
		listOpts.Name = c.Name
	}
	if c.Repository != "" {
		listOpts.Repository = c.Repository
	}

	return listOpts
}

func (c *ListCmd) fetchPipelines(ctx context.Context, f *factory.Factory, org string, listOpts *buildkite.PipelineListOptions) ([]buildkite.Pipeline, error) {
	var allPipelines []buildkite.Pipeline

	for page := 1; len(allPipelines) < c.Limit; page++ {
		listOpts.Page = page
		listOpts.PerPage = min(pageSize, c.Limit-len(allPipelines))

		pipelines, _, err := f.RestAPIClient.Pipelines.List(ctx, org, listOpts)
		if err != nil {
			return nil, err
		}

		if len(pipelines) == 0 {
			break
		}

		allPipelines = append(allPipelines, pipelines...)

		if len(pipelines) < listOpts.PerPage {
			break
		}
	}

	return allPipelines, nil
}

func (c *ListCmd) displayPipelines(pipelines []buildkite.Pipeline) error {
	format := output.Format(c.Output)
	if format != output.FormatText {
		return output.Write(os.Stdout, pipelines, format)
	}

	for _, pipeline := range pipelines {
		if pipeline.Name != "" {
			name := strings.TrimSpace(pipeline.Name)
			if name != "" {
				fmt.Println(name)
			}
		}
	}

	return nil
}
