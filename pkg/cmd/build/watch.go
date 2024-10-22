package build

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/job"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdBuildWatch(f *factory.Factory) *cobra.Command {
	var pipeline, branch string
	var intervalSeconds int

	cmd := cobra.Command{
		Use:   "watch [number] [flags]",
		Short: "Watch a build's progress in real-time",
		Args:  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			Watch a build's progress in real-time.

			You can pass an optional build number to watch. If omitted, the most recent build on the current branch will be watched.
		`),
		Example: heredoc.Doc(`
			# Watch the most recent build for the current branch
			$ bk build watch

			# Watch a specific build
			$ bk build watch 429

			# Watch the most recent build on a specific branch
			$ bk build watch -b feature-x

			# Watch a build on a specific pipeline
			$ bk build watch -p my-pipeline

			# Set a custom polling interval (in seconds)
			$ bk build watch --interval 5
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			optionsResolver := options.AggregateResolver{
				options.ResolveBranchFromFlag(branch),
				options.ResolveBranchFromRepository(f.GitRepository),
			}

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("no running builds found")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Watching build %d on %s/%s\n", bld.BuildNumber, bld.Organization, bld.Pipeline)

			ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					b, _, err := f.RestAPIClient.Builds.Get(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
					if err != nil {
						return err
					}

					summary := buildSummaryWithJobs(b)
					fmt.Fprintf(cmd.OutOrStdout(), "\033[2J\033[H%s\n", summary) // Clear screen and move cursor to top-left

					if b.FinishedAt != nil {
						return nil
					}
				case <-cmd.Context().Done():
					return nil
				}
			}
		},
	}

	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to watch. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "The branch to watch builds for.")
	cmd.Flags().IntVar(&intervalSeconds, "interval", 1, "Polling interval in seconds")

	return &cmd
}

func buildSummaryWithJobs(b buildkite.Build) string {
	summary := build.BuildSummary(b)

	if len(b.Jobs) > 0 {
		summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\nJobs")
		for _, j := range b.Jobs {
			if j.Type == "script" {
				summary += job.JobSummary(job.Job(j))
			}
		}
	}

	return summary
}
