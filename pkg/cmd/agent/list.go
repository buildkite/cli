package agent

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	var name, version, hostname, state string
	var perpage, limit int

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "List agents",
		Long: heredoc.Doc(`
			List connected agents for the current organization.

			By default, shows up to 100 agents. Use filters to narrow results, or increase the number of agents displayed with --limit.
		`),
		Example: heredoc.Doc(`
			# List all agents
			$ bk agent list

			# List agents with JSON output
			$ bk agent list --output json

			# List only running agents (currently executing jobs)
			$ bk agent list --state running

			# List only idle agents (connected but not running jobs)
			$ bk agent list --state idle

			# List only paused agents
			$ bk agent list --state paused

			# Filter agents by hostname
			$ bk agent list --hostname my-server-01

			# Combine state and hostname filters
			$ bk agent list --state idle --hostname my-server-01

			# Multiple filters with output format
			$ bk agent list --state running --version 3.107.2 --output json
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			if format != output.FormatText {
				agents := []buildkite.Agent{}
				page := 1

				for len(agents) < limit && page < 50 {
					opts := buildkite.AgentListOptions{
						Name:     name,
						Hostname: hostname,
						Version:  version,
						ListOptions: buildkite.ListOptions{
							Page:    page,
							PerPage: perpage,
						},
					}

					pageAgents, _, err := f.RestAPIClient.Agents.List(cmd.Context(), f.Config.OrganizationSlug(), &opts)
					if err != nil {
						return err
					}

					if len(pageAgents) == 0 {
						break
					}

					filtered := filterAgentsByState(pageAgents, state)
					agents = append(agents, filtered...)
					page++
				}

				if len(agents) > limit {
					agents = agents[:limit]
				}

				return output.Write(cmd.OutOrStdout(), agents, format)
			}

			loader := func(page int) tea.Cmd {
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

					agents, resp, err := f.RestAPIClient.Agents.List(cmd.Context(), f.Config.OrganizationSlug(), &opts)
					if err != nil {
						return err
					}

					filtered := filterAgentsByState(agents, state)

					items := make([]agent.AgentListItem, len(filtered))
					for i, a := range filtered {
						a := a
						items[i] = agent.AgentListItem{Agent: a}
					}

					return agent.NewAgentItemsMsg(items, resp.LastPage)
				}
			}

			model := agent.NewAgentList(loader, 1, perpage)

			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter agents by their name")
	cmd.Flags().StringVar(&version, "version", "", "Filter agents by their version")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Filter agents by their hostname")
	cmd.Flags().StringVar(&state, "state", "", "Filter agents by state (running, idle, paused)")
	cmd.Flags().IntVar(&perpage, "per-page", 30, "Number of agents per page")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of agents to return")
	output.AddFlags(cmd.Flags())

	return &cmd
}

// filterAgentsByState filters agents by their state (running, idle, paused)
// Returns all agents if state is empty
func filterAgentsByState(agents []buildkite.Agent, state string) []buildkite.Agent {
	if state == "" {
		return agents
	}

	filtered := make([]buildkite.Agent, 0, len(agents))
	for _, a := range agents {
		switch state {
		case "running":
			// Agent is running if it has a job
			if a.Job != nil {
				filtered = append(filtered, a)
			}
		case "idle":
			if a.Job == nil && (a.Paused == nil || !*a.Paused) {
				filtered = append(filtered, a)
			}
		case "paused":
			if a.Paused != nil && *a.Paused {
				filtered = append(filtered, a)
			}
		}
	}
	return filtered
}
