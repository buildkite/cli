package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdAgentView(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view <agent>",
		Args:                  cobra.ExactArgs(1),
		Short:                 "View details of an agent",
		Long: heredoc.Doc(`
			View details of an agent.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, id := parseAgentArg(args[0], f.Config)

			if web {
				url := fmt.Sprintf("https://buildkite.com/organizations/%s/agents/%s", org, id)
				fmt.Printf("Opening %s in your browser\n", url)
				return browser.OpenURL(url)
			}

			l := io.NewPendingCommand(func() tea.Msg {
				agentData, _, err := f.RestAPIClient.Agents.Get(org, id)

				if err != nil {
					return err
				}

				// Obtain agent table data output and return
				agentTable := agent.AgentDataTable(agentData)
				return io.PendingOutput(agentTable)
			}, "Loading agent")

			p := tea.NewProgram(l)
			_, err := p.Run()

			return err
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open agent in a browser")

	return &cmd
}
