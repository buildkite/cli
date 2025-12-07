package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	stateRunning = "running"
	stateIdle    = "idle"
	statePaused  = "paused"
)

var validStates = []string{stateRunning, stateIdle, statePaused}

type ListCmd struct {
	Name     string   `help:"Filter agents by their name"`
	Version  string   `help:"Filter agents by their version"`
	Hostname string   `help:"Filter agents by their hostname"`
	State    string   `help:"Filter agents by state (running, idle, paused)"`
	Tags     []string `help:"Filter agents by tags"`
	PerPage  int      `help:"Number of agents per page" default:"30"`
	Limit    int      `help:"Maximum number of agents to return" default:"100"`
	Output   string   `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ListCmd) Help() string {
	return `By default, shows up to 100 agents. Use filters to narrow results, or increase the number of agents displayed with --limit.

Examples:
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

  # Filter agents by tags
  $ bk agent list --tags queue=default

  # Filter agents by multiple tags (all must match)
  $ bk agent list --tags queue=default --tags os=linux

  # Multiple filters with output format
  $ bk agent list --state running --version 3.107.2 --output json`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	if err := validateState(c.State); err != nil {
		return err
	}

	format := output.Format(c.Output)

	// Skip TUI when using non-text format (JSON/YAML)
	if format != output.FormatText {
		agents := []buildkite.Agent{}
		page := 1

		for len(agents) < c.Limit && page < 50 {
			opts := buildkite.AgentListOptions{
				Name:     c.Name,
				Hostname: c.Hostname,
				Version:  c.Version,
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: c.PerPage,
				},
			}

			pageAgents, _, err := f.RestAPIClient.Agents.List(ctx, f.Config.OrganizationSlug(), &opts)
			if err != nil {
				return err
			}

			if len(pageAgents) == 0 {
				break
			}

			filtered := filterAgents(pageAgents, c.State, c.Tags)
			agents = append(agents, filtered...)
			page++
		}

		if len(agents) > c.Limit {
			agents = agents[:c.Limit]
		}

		return output.Write(os.Stdout, agents, format)
	}

	loader := func(page int) tea.Cmd {
		return func() tea.Msg {
			opts := buildkite.AgentListOptions{
				Name:     c.Name,
				Hostname: c.Hostname,
				Version:  c.Version,
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: c.PerPage,
				},
			}

			agents, resp, err := f.RestAPIClient.Agents.List(ctx, f.Config.OrganizationSlug(), &opts)
			if err != nil {
				return err
			}

			filtered := filterAgents(agents, c.State, c.Tags)

			items := make([]agent.AgentListItem, len(filtered))
			for i, a := range filtered {
				a := a
				items[i] = agent.AgentListItem{Agent: a}
			}

			return agent.NewAgentItemsMsg(items, resp.LastPage)
		}
	}

	model := agent.NewAgentList(loader, 1, c.PerPage, f.Quiet)

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func validateState(state string) error {
	if state == "" {
		return nil
	}

	normalized := strings.ToLower(state)
	for _, valid := range validStates {
		if normalized == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid state %q: must be one of %s, %s, or %s", state, stateRunning, stateIdle, statePaused)
}

func filterAgents(agents []buildkite.Agent, state string, tags []string) []buildkite.Agent {
	filtered := make([]buildkite.Agent, 0, len(agents))
	for _, a := range agents {
		if matchesState(a, state) && matchesTags(a, tags) {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func matchesState(a buildkite.Agent, state string) bool {
	if state == "" {
		return true
	}

	normalized := strings.ToLower(state)
	switch normalized {
	case stateRunning:
		return a.Job != nil
	case stateIdle:
		return a.Job == nil && (a.Paused == nil || !*a.Paused)
	case statePaused:
		return a.Paused != nil && *a.Paused
	default:
		return false
	}
}

func matchesTags(a buildkite.Agent, tags []string) bool {
	if len(tags) == 0 {
		return true
	}

	for _, tag := range tags {
		if !hasTag(a.Metadata, tag) {
			return false
		}
	}
	return true
}

func hasTag(metadata []string, tag string) bool {
	for _, meta := range metadata {
		if meta == tag {
			return true
		}
	}
	return false
}
