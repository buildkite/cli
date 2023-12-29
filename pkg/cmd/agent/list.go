package agent

import (
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	var name, version, hostname string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "List agents",
		Long: heredoc.Doc(`
			List all connected agents for the current organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			l := io.NewPendingCommand(func() tea.Msg {
				var alo buildkite.AgentListOptions

				if name != "" || version != "" || hostname != "" {
					alo = buildkite.AgentListOptions{
						Name:     name,
						Version:  version,
						Hostname: hostname,
					}
				}

				// Obtain agent list
				agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &alo)
				if err != nil {
					return err
				}

				if len(agents) == 0 {
					return io.PendingOutput("No connected agents")
				}

				var rows []table.Row
				maxLengths := map[string]int{"ID": 0, "Name": 0, "Hostname": 0, "Status": 0, "Tags": 0, "Version": 0}

				for _, agent := range agents {
					agentRowData := map[string]string{
						"ID":       *agent.ID,
						"Name":     *agent.Name,
						"Hostname": *agent.Hostname,
						"Status":   *agent.ConnectedState,
						"Tags":     strings.Join(agent.Metadata, ", "),
						"Version":  *agent.Version,
					}

					// If any length of an agents values are longer than its current maximum length, update it
					for key, value := range agentRowData {
						if len(value) > maxLengths[key] {
							maxLengths[key] = len(value)
						}
					}

					// Append row to table.Row list
					rows = append(rows, []string{agentRowData["ID"], agentRowData["Name"], agentRowData["Hostname"], agentRowData["Status"], agentRowData["Tags"], agentRowData["Version"]})
				}

				columns := []table.Column{
					{Title: "ID", Width: maxLengths["ID"]},
					{Title: "Name", Width: maxLengths["Name"]},
					{Title: "Hostname", Width: maxLengths["Hostname"]},
					{Title: "Status", Width: maxLengths["Status"]},
					{Title: "Tags", Width: maxLengths["Tags"]},
					{Title: "Version", Width: maxLengths["Version"] + 1},
				}

				// Set the selected style to default
				styles := table.DefaultStyles()
				styles.Selected = lipgloss.NewStyle()
				table := table.New(table.WithColumns(columns), table.WithRows(rows), table.WithHeight(len(rows)), table.WithStyles(styles))

				return io.PendingOutput(table.View())
			}, "Loading agents")

			_, err := tea.NewProgram(l).Run()
			return err
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter agents by their name")
	cmd.Flags().StringVar(&version, "version", "", "Filter agents by their agent version")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Filter agents by their hostname")

	return &cmd
}
