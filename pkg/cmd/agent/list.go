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

				agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &alo)
				if err != nil {
					return err
				}

				if len(agents) == 0 {
					return io.PendingOutput("No connected agents")
				}

				var rows []table.Row
				maxID, maxName, maxStatus, maxTags := 0, 0, 0, 0
				for _, agent := range agents {
					if max := len(*agent.ID); max > maxID {
						maxID = max
					}
					if max := len(*agent.Name); max > maxName {
						maxName = max
					}
					if max := len(*agent.ConnectedState); max > maxStatus {
						maxStatus = max
					}
					tags := strings.Join(agent.Metadata, ", ")
					if max := len(tags); max > maxTags {
						maxTags = max
					}
					row := []string{
						*agent.ID, *agent.Name, *agent.ConnectedState, tags,
					}

					rows = append(rows, row)
				}
				columns := []table.Column{
					{Title: "ID", Width: maxID},
					{Title: "Name", Width: maxName},
					{Title: "Status", Width: maxStatus},
					{Title: "Tags", Width: maxTags},
				}
				// set the selected style to default
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
	cmd.Flags().StringVar(&version, "hostname", "", "Filter agents by their hostname")

	return &cmd
}
