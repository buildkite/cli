package build

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdBuildCancel(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "cancel <number> [pipeline] [flags]",
		Short:                 "Cancels a build.",
		Args:                  cobra.MinimumNArgs(1),
		Long: heredoc.Doc(`
			Cancels the specified build.

			It accepts a build number and a pipeline slug  as an argument.
			The pipeline can be a {pipeline_slug} or in the format {org_slug}/{pipeline_slug}.
			If the pipeline argument is omitted, it will be resolved using the current directory.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildId := args[0]
			resolvers := resolver.NewAggregateResolver(
				resolver.ResolveFromPositionalArgument(args, 1, f.Config),
				resolver.ResolveFromConfig(f.LocalConfig),
				resolver.ResolveFromRepository(f),
			)
			var pipeline pipeline.Pipeline
			r := io.NewPendingCommand(func() tea.Msg {
				p, err := resolvers.Resolve(context.Background())
				if err != nil {
					return err
				}
				pipeline = *p

				return io.PendingOutput(fmt.Sprintf("Resolved pipeline to: %s", pipeline.Name))
			}, "Resolving pipeline")
			p := tea.NewProgram(r)
			finalModel, err := p.Run()
			if err != nil {
				return err
			}
			if finalModel.(io.Pending).Err != nil {
				return finalModel.(io.Pending).Err
			}
			return cancelBuild(pipeline.Org, pipeline.Name, buildId, web, f)
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been cancelled.")
	cmd.Flags().SortFlags = false
	return &cmd
}

func cancelBuild(org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	l := io.NewPendingCommand(func() tea.Msg {
		build, err := f.RestAPIClient.Builds.Cancel(org, pipeline, buildId)
		if err != nil {
			return err
		}

		if err = openBuildInBrowser(web, *build.WebURL); err != nil {
			return err
		}

		return io.PendingOutput(renderResult(fmt.Sprintf("Cancelling build: %s\n", *build.WebURL)))

	}, fmt.Sprintf("Cancelling build #%s from pipeline %s", buildId, pipeline))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}
