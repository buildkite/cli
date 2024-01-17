package agent

import (
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
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
			opts := buildkite.AgentListOptions{
				Name:     name,
				Hostname: hostname,
				Version:  version,
			}
			loader := func() tea.Msg {
				agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &opts)
				items := make(agent.NewAgentItemsMsg, len(agents))

				if err != nil {
					return err
				}

				for i, a := range agents {
					a := a
					items[i] = agent.AgentListItem{
						Agent: &a,
					}
				}
				return items
			}

			stopper := func(id string, force bool) error {
				_, err := f.RestAPIClient.Agents.Stop(f.Config.Organization, id, force)
				return err
			}

			model := agent.NewAgentList(loader, stopper)

			p := tea.NewProgram(model, tea.WithAltScreen())

			if _, err := p.Run(); err != nil {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter agents by their name")
	cmd.Flags().StringVar(&version, "version", "", "Filter agents by their agent version")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Filter agents by their hostname")

	return &cmd
}
