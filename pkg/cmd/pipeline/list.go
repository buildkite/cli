package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
)

const (
	maxPipelineLimit = 3000
	pageSize         = 100
)

type pipelineListOptions struct {
	name       string
	repository string
	limit      int
}

func NewCmdPipelineList(f *factory.Factory) *cobra.Command {
	var opts pipelineListOptions

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [flags]",
		Short:                 "List pipelines",
		Long: heredoc.Doc(`
			List pipelines with optional filtering.
			This command lists all pipelines in the current organization that match
			the specified criteria. You can filter by pipeline name or repository URL.
		`),
		Example: heredoc.Doc(`
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
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			if opts.limit > maxPipelineLimit {
				return fmt.Errorf("limit cannot exceed %d pipelines (requested: %d)", maxPipelineLimit, opts.limit)
			}

			return runPipelineList(cmd.Context(), f, &opts, format)
		},
	}

	cmd.Flags().StringVar(&opts.name, "name", "", "Filter pipelines by name (supports partial matches, case insensitive)")
	cmd.Flags().StringVar(&opts.repository, "repo", "", "Filter pipelines by repository URL (supports partial matches, case insensitive)")
	cmd.Flags().IntVar(&opts.limit, "limit", 100, fmt.Sprintf("Maximum number of pipelines to return (max: %d)", maxPipelineLimit))

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false

	return &cmd
}

func runPipelineList(ctx context.Context, f *factory.Factory, opts *pipelineListOptions, format output.Format) error {
	org := f.Config.OrganizationSlug()
	if org == "" {
		return fmt.Errorf("no organization configured. Use 'bk configure' to set up your organization")
	}

	listOpts := pipelineListOptionsFromFlags(opts)

	var pipelines []buildkite.Pipeline
	var err error

	err = bk_io.SpinWhile("Loading pipelines", func() {
		pipelines, err = fetchPipelines(ctx, f, org, opts, listOpts)
	})
	if err != nil {
		return fmt.Errorf("failed to list pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		fmt.Fprintln(os.Stdout, "No pipelines found matching the specified criteria.")
		return nil
	}

	return displayPipelines(pipelines, format)
}

func pipelineListOptionsFromFlags(opts *pipelineListOptions) *buildkite.PipelineListOptions {
	listOpts := &buildkite.PipelineListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: pageSize,
		},
	}

	if opts.name != "" {
		listOpts.Name = opts.name
	}
	if opts.repository != "" {
		listOpts.Repository = opts.repository
	}

	return listOpts
}

func fetchPipelines(ctx context.Context, f *factory.Factory, org string, opts *pipelineListOptions, listOpts *buildkite.PipelineListOptions) ([]buildkite.Pipeline, error) {
	var allPipelines []buildkite.Pipeline

	for page := 1; len(allPipelines) < opts.limit; page++ {
		listOpts.Page = page
		listOpts.PerPage = min(pageSize, opts.limit-len(allPipelines))

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

func displayPipelines(pipelines []buildkite.Pipeline, format output.Format) error {
	if format != output.FormatText {
		return output.Write(os.Stdout, pipelines, format)
	}

	// For text format, output pipeline names (one per line for easy piping to other commands)
	for _, pipeline := range pipelines {
		if pipeline.Name != "" {
			// Clean the pipeline name for output (remove any whitespace)
			name := strings.TrimSpace(pipeline.Name)
			if name != "" {
				fmt.Println(name)
			}
		}
	}

	return nil
}