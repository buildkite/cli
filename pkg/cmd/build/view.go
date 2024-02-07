package build

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdBuildView(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view <number> <pipeline> [flags]",
		Args:                  cobra.ExactArgs(2),
		Short:                 "View build information.",
		Long: heredoc.Doc(`
			View a build's information.

			It accepts a build number and a pipeline slug  as an argument.
			The pipeline can be a {pipeline_slug}, {org_slug}/{pipeline_slug} or a full URL to the pipeline.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildId := args[0]
			org, pipeline := parsePipelineArg(args[1], f.Config)

			l := io.NewPendingCommand(func() tea.Msg {
				var buildUrl string

				b, _, err := f.RestAPIClient.Builds.Get(org, pipeline, buildId, &buildkite.BuildsListOptions{})
				if err != nil {
					return err
				}

				if web {
					buildUrl = fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", org, pipeline, *b.Number)
					fmt.Printf("Opening %s in your browser\n\n", buildUrl)
					time.Sleep(1 * time.Second)
					err = browser.OpenURL(buildUrl)
					if err != nil {
						fmt.Println("Error opening browser: ", err)
					}
				}

				// Obtain build summary and return
				summary := build.BuildSummary(b)
				return io.PendingOutput(summary)
			}, "Loading build information")

			p := tea.NewProgram(l)
			_, err := p.Run()

			return err
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")

	return &cmd
}
