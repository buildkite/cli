package agent

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
	Output   string   `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:"json,yaml,text"`
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
	f, err := factory.New()
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	if err := validateState(c.State); err != nil {
		return err
	}

	format := output.Format(c.Output)

	agents := []buildkite.Agent{}
	page := 1
	hasMore := false
	var previousFirstAgentID string

	for len(agents) < c.Limit {
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

		if page > 1 && len(pageAgents) > 0 && pageAgents[0].ID == previousFirstAgentID {
			return fmt.Errorf("API returned duplicate page content at page %d, stopping pagination to prevent infinite loop", page)
		}
		if len(pageAgents) > 0 {
			previousFirstAgentID = pageAgents[0].ID
		}

		filtered := filterAgents(pageAgents, c.State, c.Tags)
		agents = append(agents, filtered...)

		// If this was a full page, there might be more results
		// We'll check after breaking from the loop if we hit the limit with a full page
		if len(pageAgents) < c.PerPage {
			break
		}

		// Check if we've hit the limit before fetching more
		if len(agents) >= c.Limit {
			// We hit the limit with a full page, so there are likely more results
			hasMore = true
			break
		}

		page++
	}

	totalFetched := len(agents)
	if len(agents) > c.Limit {
		agents = agents[:c.Limit]
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, agents, format)
	}

	if len(agents) == 0 {
		fmt.Println("No agents found")
		return nil
	}

	headers := []string{"State", "Name", "Version", "Queue", "Hostname"}
	rows := make([][]string, len(agents))
	for i, agent := range agents {
		queue := extractQueue(agent.Metadata)
		state := displayState(agent)
		rows[i] = []string{
			state,
			agent.Name,
			agent.Version,
			queue,
			agent.Hostname,
		}
	}

	columnStyles := map[string]string{
		"state":    "bold",
		"name":     "bold",
		"hostname": "dim",
		"version":  "italic",
		"queue":    "italic",
	}
	table := output.Table(headers, rows, columnStyles)

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() {
		_ = cleanup()
	}()

	totalDisplay := fmt.Sprintf("%d", totalFetched)
	if hasMore {
		totalDisplay = fmt.Sprintf("%d+", totalFetched)
	}
	fmt.Fprintf(writer, "Showing %d of %s agents in %s\n\n", len(agents), totalDisplay, f.Config.OrganizationSlug())
	fmt.Fprint(writer, table)

	return nil
}

func validateState(state string) error {
	if state == "" {
		return nil
	}

	normalized := strings.ToLower(state)
	if slices.Contains(validStates, normalized) {
		return nil
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
	return slices.Contains(metadata, tag)
}

func extractQueue(metadata []string) string {
	for _, m := range metadata {
		if after, ok := strings.CutPrefix(m, "queue="); ok {
			return after
		}
	}
	return "default"
}

func displayState(a buildkite.Agent) string {
	if a.Job != nil {
		return stateRunning
	}

	if a.Paused != nil && *a.Paused {
		return statePaused
	}

	return stateIdle
}
