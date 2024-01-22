package agent

import (
	//"fmt"
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
	var perpage int

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "List agents",
		Long: heredoc.Doc(`
			List all connected agents for the current organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := func(page, perpage int) tea.Cmd {
				return func() tea.Msg {
					opts := buildkite.AgentListOptions{
						Name:     name,
						Hostname: hostname,
						Version:  version,
						ListOptions: buildkite.ListOptions{
							Page:    page,
							PerPage: perpage,
						},
					}

					agents, resp, err := f.RestAPIClient.Agents.List(f.Config.Organization, &opts)
					items := make([]agent.AgentListItem, len(agents))

					if err != nil {
						return err
					}

					for i, a := range agents {
						a := a
						items[i] = agent.AgentListItem{
							Agent: &a,
						}
					}

					// If initial load, return agents with page info
					return agent.NewAgentItemsMsg{
						Items:    items,
						LastPage: resp.LastPage,
					}
				}
			}

			model := agent.NewAgentList(loader, 1, perpage)

			p := tea.NewProgram(model)

			if _, err := p.Run(); err != nil {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter agents by their name")
	cmd.Flags().StringVar(&version, "version", "", "Filter agents by their agent version")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Filter agents by their hostname")
	cmd.Flags().IntVar(&perpage, "per-page", 30, "Number of agents to fetch per API call")

	return &cmd
}
