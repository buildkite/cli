package build

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdBuildNew(f *factory.Factory) *cobra.Command {
	var message string
	var commit string
	var branch string
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "new [pipeline] [flags]",
		Short:                 "Creates a new pipeline build",
		Args:                  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			Creates a new build for the specified pipeline and output the URL to the build.

			It accepts {pipeline_slug}, {org_slug}/{pipeline_slug} or a full URL to the pipeline as an argument.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvers := pipeline.NewAggregateResolver(
				pipelineResolverPositionArg(args, f.Config),
				pipeline.ResolveFromPath("", f.Config.Organization, f.RestAPIClient),
			)
			var pipeline pipeline.Pipeline
			r := io.NewPendingCommand(func() tea.Msg {
				p, err := resolvers.Resolve()
				if err != nil {
					return err
				}
				pipeline = *p

				return io.PendingOutput(fmt.Sprintf("Resolved pipeline to: %s", pipeline.Name))
			}, "Resolving pipeline")
			p := tea.NewProgram(r)
			_, err := p.Run()
			if err != nil {
				return err
			}
			return newBuild(pipeline.Org, pipeline.Name, f, message, commit, branch, web)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Description of the build. If left blank, the commit message will be used once the build starts.")
	cmd.Flags().StringVarP(&commit, "commit", "c", "HEAD", "The commit to build.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "The branch to build. Defaults to the default branch of the pipeline.")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")
	cmd.Flags().SortFlags = false
	return &cmd
}

func pipelineResolverPositionArg(args []string, conf *config.Config) pipeline.PipelineResolverFn {
	return func() (*pipeline.Pipeline, error) {
		// if args does not have values, skip this resolver
		if len(args) < 1 {
			return nil, nil
		}

		org, name := parsePipelineArg(args[0], conf)
		// if we could not parse the pipeline from the arg then return no pipeline or error, to pass indicate to pass to
		// the next resolver in the chain
		if org == "" || name == "" {
			return nil, nil
		}

		return &pipeline.Pipeline{Name: name, Org: org}, nil
	}
}

func newBuild(org string, pipeline string, f *factory.Factory, message string, commit string, branch string, web bool) error {
	l := io.NewPendingCommand(func() tea.Msg {

		if len(branch) == 0 {
			p, _, err := f.RestAPIClient.Pipelines.Get(org, pipeline)
			if err != nil {
				return err
			}
			branch = *p.DefaultBranch
		}

		newBuild := buildkite.CreateBuild{
			Message: message,
			Commit:  commit,
			Branch:  branch,
		}

		build, _, err := f.RestAPIClient.Builds.Create(org, pipeline, &newBuild)
		if err != nil {
			return err
		}

		if err = openBuildInBrowser(web, *build.WebURL); err != nil {
			return err
		}

		return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
			lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Build created: %s\n", *build.WebURL))))

	}, fmt.Sprintf("Starting new build for %s", pipeline))
	p := tea.NewProgram(l)
	_, err := p.Run()
	return err
}
