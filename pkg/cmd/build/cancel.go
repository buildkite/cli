package build

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdBuildCancel(f *factory.Factory) *cobra.Command {
	var web bool
	var pipeline string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "cancel <number> [flags]",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Cancel a build.",
		Long: heredoc.Doc(`
			Cancel the given build.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("could not resolve a build")
			}

			return cancelBuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), web, f)
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been cancelled.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to cancel a build on. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
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

		return io.PendingOutput(renderResult(fmt.Sprintf("Build canceled: %s", *build.WebURL)))
	}, fmt.Sprintf("Cancelling build #%s from pipeline %s", buildId, pipeline))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}
