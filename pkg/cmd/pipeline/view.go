package pipeline

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	view "github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdPipelineView(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view [pipeline] [flags]",
		Short:                 "View a pipeline",
		Args:                  cobra.MaximumNArgs(1),
		Long:                  "View information about a pipeline.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := resolver.NewAggregateResolver(
				resolver.ResolveFromPositionalArgument(args, 0, f.Config),
				resolver.ResolveFromConfig(f.Config, resolver.PickOne),
				resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOne)),
			)

			pipeline, err := pipelineRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}

			slug := fmt.Sprintf("%s/%s", pipeline.Org, pipeline.Name)

			if web {
				return browser.OpenURL(fmt.Sprintf("https://buildkite.com/%s", slug))
			}

			resp, err := graphql.GetPipeline(cmd.Context(), f.GraphQLClient, slug)
			if err != nil {
				return err
			}
			if resp == nil || resp.Pipeline == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Could not find pipeline: %s\n", slug)
				return nil
			}

			var output strings.Builder

			err = view.RenderPipeline(&output, *resp.Pipeline)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", output.String())

			return nil
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the pipeline in a web browser.")

	return &cmd
}
