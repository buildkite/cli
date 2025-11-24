package build

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/build/view/shared"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

type WatchOptions struct {
	Pipeline        string
	Branch          string
	IntervalSeconds int
}

func (o *WatchOptions) Validate() error {
	v := validation.New()
	v.AddRule("IntervalSeconds", validation.MinValue(1))

	if o.Pipeline != "" {
		v.AddRule("Pipeline", validation.Slug)
	}

	return v.Validate(map[string]interface{}{
		"Pipeline":        o.Pipeline,
		"IntervalSeconds": o.IntervalSeconds,
	})
}

func NewCmdBuildWatch(f *factory.Factory) *cobra.Command {
	opts := &WatchOptions{
		IntervalSeconds: 1, // default value
	}

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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Validate command options
			if err := opts.Validate(); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get pipeline from persistent flag
			opts.Pipeline, _ = cmd.Flags().GetString("pipeline")

			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(opts.Pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
			)

			optionsResolver := options.AggregateResolver{
				options.ResolveBranchFromFlag(opts.Branch),
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

			ticker := time.NewTicker(time.Duration(opts.IntervalSeconds) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					b, _, err := f.RestAPIClient.Builds.Get(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
					if err != nil {
						return err
					}

					summary := shared.BuildSummaryWithJobs(&b)
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

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.ReadBuilds),
	}

	// Pipeline flag now inherited from parent command
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "The branch to watch builds for.")
	cmd.Flags().IntVar(&opts.IntervalSeconds, "interval", opts.IntervalSeconds, "Polling interval in seconds")

	return &cmd
}
