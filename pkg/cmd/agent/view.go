package agent

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
				agent, _, err := f.RestAPIClient.Agents.Get(org, id)
				if err != nil {
					return err
				}

				tableOut := &bytes.Buffer{}
				bold := lipgloss.NewStyle().Bold(true)
				fmt.Fprint(tableOut, bold.Render(*agent.Name))
				t := table.New().Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
					return lipgloss.NewStyle().PaddingRight(2)
				})
				t.Row("ID", *agent.ID)
				t.Row("State", bold.Render(*agent.ConnectedState))
				t.Row("Version", *agent.Version)
				t.Row("Hostname", *agent.Hostname)
				// t.Row("PID", *agent.)
				t.Row("User Agent", *agent.UserAgent)
				t.Row("IP Address", *agent.IPAddress)
				// t.Row("OS", *agent.)
				t.Row("Connected", agent.CreatedAt.UTC().Format(time.RFC1123Z))
				// t.Row("Stopped By", *agent.CreatedAt)
				t.Row("Metadata", strings.Join(agent.Metadata, ","))

				fmt.Fprint(tableOut, t.Render())
				return io.PendingOutput(tableOut.String())
			}, "Loading agent")

			p := tea.NewProgram(l)
			_, err := p.Run()

			return err
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open agent in a browser")

	return &cmd
}
