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

func NewCmdBuildRebuild(f *factory.Factory) *cobra.Command {
	var web bool
	var pipeline string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "rebuild [number] [flags]",
		Short:                 "Rebuild a build.",
		Args:                  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			Rebuild a build.
			The web URL to the build will be printed to stdout.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("could not resolve a build")
			}

			return rebuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), web, f)
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	cmd.Flags().SortFlags = false
	return &cmd
}

func rebuild(org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	l := io.NewPendingCommand(func() tea.Msg {
		build, err := f.RestAPIClient.Builds.Rebuild(org, pipeline, buildId)
		if err != nil {
			return err
		}

		if err = openBuildInBrowser(web, *build.WebURL); err != nil {
			return err
		}

		return io.PendingOutput(renderResult(fmt.Sprintf("Build created: %s", *build.WebURL)))
	}, fmt.Sprintf("Rerunning build #%s for pipeline %s", buildId, pipeline))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}
