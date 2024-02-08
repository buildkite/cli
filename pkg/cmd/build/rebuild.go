package build

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdBuildRebuild(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "rebuild <number> <pipeline> [flags]",
		Short:                 "Reruns a build.",
		Args:                  cobra.ExactArgs(2),
		Long: heredoc.Doc(`
			Runs a new build from the specified build number and pipeline and outputs the URL to the new build.

			It accepts {pipeline_slug}, {org_slug}/{pipeline_slug} or a full URL to the pipeline as an argument.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildId := args[0]
			org, pipeline := parsePipelineArg(args[1], f.Config)
			return rebuild(org, pipeline, buildId, web, f)
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")
	cmd.Flags().SortFlags = false
	return &cmd
}

func rebuild(org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	l := io.NewPendingCommand(func() tea.Msg {
		build, err := f.RestAPIClient.Builds.Rebuild(org, pipeline, buildId)
		if err != nil {
			return err
		}

		buildUrl := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", org, pipeline, *build.Number)

		if web {
			fmt.Printf("Opening %s in your browser\n", buildUrl)
			err = browser.OpenURL(buildUrl)
			if err != nil {
				fmt.Println("Error opening browser: ", err)
			}
		}

		return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
			lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Build created: %s\n", buildUrl))))

	}, fmt.Sprintf("Rerunning build #%s for pipeline %s", buildId, pipeline))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}
