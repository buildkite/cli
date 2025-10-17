package pipeline

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	view "github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
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
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			pipelineRes := resolver.NewAggregateResolver(
				resolver.ResolveFromPositionalArgument(args, 0, f.Config),
				resolver.ResolveFromConfig(f.Config, resolver.PickOne),
				resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOne, f.GitRepository != nil)),
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

			if format != output.FormatText {
				return output.Write(cmd.OutOrStdout(), resp.Pipeline, format)
			}

			var pipelineOutput strings.Builder

			err = view.RenderPipeline(&pipelineOutput, *resp.Pipeline)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", pipelineOutput.String())

			return nil
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the pipeline in a web browser.")

	output.AddFlags(cmd.Flags())
	return &cmd
}
