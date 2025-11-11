package agent

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/graphql"
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
				agents, err := fetchAgents(cmd.Context(), f, name, hostname, version, state, limit, perpage)
				if err != nil {
					return err
				}
				return output.Write(cmd.OutOrStdout(), agents, format)
			}

			loader := func(page int) tea.Cmd {
				return func() tea.Msg {
					if state != "" {
						agents, err := fetchAgentsGraphQL(cmd.Context(), f, name, hostname, version, state, perpage)
						if err != nil {
							return err
						}

						items := make([]agent.AgentListItem, len(agents))
						for i, a := range agents {
							items[i] = agent.AgentListItem{Agent: a}
						}

						return agent.NewAgentItemsMsg(items, 1)
					}

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
					items := make([]agent.AgentListItem, len(agents))

					if err != nil {
						return err
					}

					for i, a := range agents {
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

func fetchAgents(ctx context.Context, f *factory.Factory, name, hostname, version, state string, limit, perpage int) ([]buildkite.Agent, error) {
	if state != "" {
		return fetchAgentsGraphQL(ctx, f, name, hostname, version, state, limit)
	}

	var agents []buildkite.Agent
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

		pageAgents, _, err := f.RestAPIClient.Agents.List(ctx, f.Config.OrganizationSlug(), &opts)
		if err != nil {
			return nil, err
		}

		if len(pageAgents) == 0 {
			break
		}

		agents = append(agents, pageAgents...)
		page++
	}

	if len(agents) > limit {
		agents = agents[:limit]
	}

	return agents, nil
}

func stateToFilters(state string) (isRunningJob *bool, paused *bool, err error) {
	switch state {
	case "running":
		running := true
		return &running, nil, nil
	case "idle":
		notRunning := false
		isPaused := false
		return &notRunning, &isPaused, nil
	case "paused":
		notRunning := false
		isPaused := true
		return &notRunning, &isPaused, nil
	default:
		return nil, nil, fmt.Errorf("invalid state: %s (must be running, idle, or paused)", state)
	}
}

func fetchAgentsGraphQL(ctx context.Context, f *factory.Factory, name, hostname, version, state string, limit int) ([]buildkite.Agent, error) {
	isRunningJob, paused, err := stateToFilters(state)
	if err != nil {
		return nil, err
	}

	var agents []buildkite.Agent
	var after *string
	const maxPages = 50

	for page := 0; len(agents) < limit && page < maxPages; page++ {
		pageSize := 100

		resp, err := graphql.ListAgents(ctx, f.GraphQLClient, f.Config.OrganizationSlug(), &pageSize, after, isRunningJob, paused)
		if err != nil {
			return nil, err
		}

		if len(resp.Organization.Agents.Edges) == 0 {
			break
		}

		for _, edge := range resp.Organization.Agents.Edges {
			if hostname != "" && (edge.Node.Hostname == nil || *edge.Node.Hostname != hostname) {
				continue
			}
			if version != "" && (edge.Node.Version == nil || *edge.Node.Version != version) {
				continue
			}
			if name != "" && edge.Node.Name != name {
				continue
			}

			agent := buildkite.Agent{
				ID:             edge.Node.Id,
				Name:           edge.Node.Name,
				Hostname:       derefString(edge.Node.Hostname),
				Version:        derefString(edge.Node.Version),
				ConnectedState: edge.Node.ConnectionState,
				Metadata:       edge.Node.MetaData,
				IPAddress:      derefString(edge.Node.IpAddress),
				UserAgent:      derefString(edge.Node.UserAgent),
			}
			if edge.Node.CreatedAt != nil {
				agent.CreatedAt = &buildkite.Timestamp{Time: *edge.Node.CreatedAt}
			}
			agents = append(agents, agent)
		}

		if !resp.Organization.Agents.PageInfo.HasNextPage {
			break
		}
		after = resp.Organization.Agents.PageInfo.EndCursor
	}

	if len(agents) > limit {
		agents = agents[:limit]
	}

	return agents, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
