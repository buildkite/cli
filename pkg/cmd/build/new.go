package build

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdBuildNew(f *factory.Factory) *cobra.Command {
	var message string
	var commit string
	var branch string
	var openWeb bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "new <pipeline> [--message <message>] [--commit <commit>] [--branch <branch>] [--web]]",
		Short:                 "Creates a new pipeline build",
		Args:                  cobra.ArbitraryArgs,
		Long: heredoc.Doc(`
			Creates a new build for the specified pipeline and output the URL to the build

			If no pipeline is specified, the pipeline to build will be resolved from the project configuration.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("build new command args here: ", args)
			switch pipelines := len(args); {
			case pipelines == 0:
				fmt.Println("Resolve pipeline name:  ", f.ProjectConfig.Pipeline)
				return newBuild(f.Config.Organization, f.ProjectConfig.Pipeline, f, message, commit, branch, openWeb)
			case pipelines == 1:
				org, pipeline := parsePipelineArg(args[0], f.Config)
				return newBuild(org, pipeline, f, message, commit, branch, openWeb)
			default:
				return errors.New("only one pipeline to build should be specified")
			}
		},
	}

	cmd.Flags().StringVar(&message, "message", "", "Description of the build. If left blank, the commit message will be used once the build starts.")
	cmd.Flags().StringVar(&commit, "commit", "HEAD", "The commit to build. Defaults to HEAD")
	cmd.Flags().StringVar(&branch, "branch", "", "The branch to build. Defaults to the default branch of the pipeline.")
	cmd.Flags().BoolVar(&openWeb, "web", false, "Open the build in a web browser after it has been created.")
	return &cmd
}

func newBuild(org string, pipeline string, f *factory.Factory, message string, commit string, branch string, openWeb bool) error {
	l := io.NewPendingCommand(func() tea.Msg {
		newBuild := buildkite.CreateBuild{
			Message: message,
			Commit:  commit,
			Branch:  branch,
		}
		build, _, err := f.RestAPIClient.Builds.Create(org, pipeline, &newBuild)
		if err != nil {
			return err
		}
		return io.PendingOutput(fmt.Sprintf("Build created: %s", *build.URL))
	}, fmt.Sprintf("Starting build for %s", pipeline))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}
