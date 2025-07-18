package cli

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/config"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"golang.org/x/sync/semaphore"
)

// Agent commands
type AgentCmd struct {
	List AgentListCmd `cmd:"" help:"List agents"`
	Stop AgentStopCmd `cmd:"" help:"Stop agents"`
	View AgentViewCmd `cmd:"" help:"View agent details"`
}

type AgentListCmd struct {
	OutputFlag   `embed:""`
	Organization string `help:"Organization slug (if omitted, uses configured organization)" placeholder:"my-org"`
	Cluster      string `help:"Cluster UUID"`
	Queue        string `help:"Queue name"`
	Meta         string `help:"Agent metadata"`
	Name         string `help:"Filter agents by their name"`
	Version      string `help:"Filter agents by their agent version"`
	Hostname     string `help:"Filter agents by their hostname"`
	PerPage      int    `help:"Number of agents to fetch per API call" default:"30"`
}

func (a *AgentListCmd) Help() string {
	return `Examples:
  # List all agents (interactive TUI)
  bk agent list
  
  # Filter agents by queue
  bk agent list --queue=deploy
  
  # Filter agents by hostname
  bk agent list --hostname=ci-server-01
  
  # List agents in a specific cluster
  bk agent list --cluster=01234567-89ab-cdef-0123-456789abcdef
  
  # Get agent list as JSON
  bk agent list --output json`
}

type AgentStopCmd struct {
	Agents []string `arg:"" help:"Agent UUIDs to stop"`
	Force  bool     `help:"Force stop the agent"`
	Limit  int      `short:"l" help:"Number of agents to stop concurrently" default:"5"`
}

type AgentViewCmd struct {
	OutputFlag `embed:""`
	Agent      string `arg:"" help:"Agent UUID to view"`
	Web        bool   `short:"w" help:"Open agent in a browser"`
	Watch      bool   `help:"Watch for changes"`
}

// Agent command implementations
func (a *AgentListCmd) Run(ctx context.Context, f *factory.Factory) error {
	a.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	org := a.Organization
	if org == "" {
		org = f.Config.OrganizationSlug()
	}

	// Use TUI by default unless structured output is requested
	if !ShouldUseStructuredOutput(f) {
		return a.runInteractive(ctx, f, org)
	}

	// Prepare list options
	opts := &buildkite.AgentListOptions{
		Name:     a.Name,
		Hostname: a.Hostname,
		Version:  a.Version,
		ListOptions: buildkite.ListOptions{
			PerPage: a.PerPage,
		},
	}
	if a.Meta != "" {
		// Meta data filters - extend existing opts
		if opts.Name == "" {
			opts.Name = a.Meta
		}
	}

	// List agents
	var agents []buildkite.Agent
	var err error
	spinErr := bk_io.SpinWhile("Loading agents", func() {
		agents, _, err = f.RestAPIClient.Agents.List(ctx, org, opts)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error listing agents: %w", err)
	}

	if len(agents) == 0 {
		if ShouldUseStructuredOutput(f) {
			return Print([]any{}, f)
		}
		fmt.Println("No agents found")
		return nil
	}

	// Output agents in requested format
	if ShouldUseStructuredOutput(f) {
		return Print(agents, f)
	}

	// Display agents
	fmt.Printf("%-40s %-15s %-20s %s\n", "ID", "State", "Hostname", "Version")
	fmt.Println(strings.Repeat("-", 90))
	for _, agent := range agents {
		hostname := agent.Hostname
		if len(hostname) > 20 {
			hostname = hostname[:17] + "..."
		}
		fmt.Printf("%-40s %-15s %-20s %s\n",
			agent.ID,
			agent.ConnectedState,
			hostname,
			agent.Version)
	}

	return nil
}

func (a *AgentListCmd) runInteractive(ctx context.Context, f *factory.Factory, org string) error {
	loader := func(page int) tea.Cmd {
		return func() tea.Msg {
			opts := buildkite.AgentListOptions{
				Name:     a.Name,
				Hostname: a.Hostname,
				Version:  a.Version,
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: a.PerPage,
				},
			}

			agents, resp, err := f.RestAPIClient.Agents.List(ctx, org, &opts)
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

	model := agent.NewAgentList(loader, 1, a.PerPage)

	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}

	return nil
}

func (a *AgentStopCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Process agents - handling both single and multiple agents
	var agentIDs []string

	// Check if reading from stdin
	if bk_io.HasDataAvailable(os.Stdin) {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				agentIDs = append(agentIDs, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading from stdin: %w", err)
		}
	} else if len(a.Agents) > 0 {
		// Use provided arguments
		agentIDs = a.Agents
	} else {
		return fmt.Errorf("must supply agents to stop")
	}

	if len(agentIDs) == 0 {
		return fmt.Errorf("no agents to stop")
	}

	// Use TUI for bulk operations (multiple agents)
	if len(agentIDs) > 1 {
		return a.runInteractiveStop(ctx, f, agentIDs)
	}

	// Single agent: stop without TUI
	var wg sync.WaitGroup
	errChan := make(chan error, len(agentIDs))
	sem := make(chan struct{}, a.Limit) // Limit concurrent operations

	for _, agentArg := range agentIDs {
		wg.Add(1)
		go func(agentArg string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			org, agentID := parseAgentArg(agentArg, f.Config)

			// Stop the agent
			var err error
			spinErr := bk_io.SpinWhile(fmt.Sprintf("Stopping agent %s", agentArg), func() {
				_, err = f.RestAPIClient.Agents.Stop(ctx, org, agentID, a.Force)
			})
			if spinErr != nil {
				errChan <- spinErr
				return
			}
			if err != nil {
				errChan <- fmt.Errorf("error stopping agent %s: %w", agentArg, err)
				return
			}

			fmt.Printf("Agent %s stopped successfully\n", agentArg)
		}(agentArg)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop %d agent(s): %v", len(errors), errors)
	}

	fmt.Printf("Successfully stopped %d agent(s)\n", len(agentIDs))
	return nil
}

func (a *AgentStopCmd) runInteractiveStop(ctx context.Context, f *factory.Factory, agentIDs []string) error {
	// Use a wait group to ensure we exit the program after all agents have finished
	var wg sync.WaitGroup
	// This semaphore is used to limit how many concurrent API requests can be sent
	sem := semaphore.NewWeighted(int64(a.Limit))

	var agents []agent.StoppableAgent
	for _, id := range agentIDs {
		if strings.TrimSpace(id) != "" {
			wg.Add(1)
			agents = append(agents, agent.NewStoppableAgent(id, stopper(ctx, id, f, a.Force, sem, &wg)))
		}
	}

	bulkAgent := agent.BulkAgent{
		Agents: agents,
	}

	p := tea.NewProgram(bulkAgent, tea.WithOutput(os.Stdout))

	// Send a quit message after all agents have stopped
	go func() {
		wg.Wait()
		p.Send(tea.Quit())
	}()

	_, err := p.Run()
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if agent.Errored() {
			return fmt.Errorf("at least one agent failed to stop")
		}
	}
	return nil
}

// stopper creates a StopFn for stopping an agent
func stopper(ctx context.Context, id string, f *factory.Factory, force bool, sem *semaphore.Weighted, wg *sync.WaitGroup) agent.StopFn {
	org, agentID := parseAgentArg(id, f.Config)
	return func() agent.StatusUpdate {
		// Before attempting to stop the agent, acquire a semaphore lock to limit parallelisation
		_ = sem.Acquire(context.Background(), 1)

		return agent.StatusUpdate{
			ID:     id,
			Status: agent.Stopping,
			// Return a new command to actually stop the agent in the API and return the status of that
			Cmd: func() tea.Msg {
				// Defer the semaphore and waitgroup release until the whole operation is completed
				defer sem.Release(1)
				defer wg.Done()
				_, err := f.RestAPIClient.Agents.Stop(ctx, org, agentID, force)
				if err != nil {
					return agent.StatusUpdate{
						ID:  id,
						Err: err,
					}
				}
				return agent.StatusUpdate{
					ID:     id,
					Status: agent.Succeeded,
				}
			},
		}
	}
}

func (a *AgentViewCmd) Run(ctx context.Context, f *factory.Factory) error {
	a.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	org, agentID := parseAgentArg(a.Agent, f.Config)

	if a.Web {
		url := fmt.Sprintf("https://buildkite.com/organizations/%s/agents/%s", org, agentID)
		fmt.Printf("Opening %s in your browser\n", url)
		return browser.OpenURL(url)
	}

	if a.Watch {
		return watchAgent(ctx, f, org, agentID)
	}

	// Get agent details
	var err error
	var agent buildkite.Agent
	spinErr := bk_io.SpinWhile("Loading agent", func() {
		agent, _, err = f.RestAPIClient.Agents.Get(ctx, org, agentID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error getting agent: %w", err)
	}

	// Output agent information in requested format
	if ShouldUseStructuredOutput(f) {
		return Print(agent, f)
	}

	// Display agent information (simplified)
	fmt.Printf("Agent: %s\n", agent.Name)
	fmt.Printf("ID: %s\n", agent.ID)
	fmt.Printf("State: %s\n", agent.ConnectedState)
	fmt.Printf("Hostname: %s\n", agent.Hostname)
	fmt.Printf("Version: %s\n", agent.Version)
	return nil
}

// parseAgentArg parses agent argument from various formats
func parseAgentArg(agent string, conf *config.Config) (string, string) {
	var org, id string
	agentIsURL := strings.Contains(agent, ":")
	agentIsSlug := !agentIsURL && strings.Contains(agent, "/")

	if agentIsURL {
		parsedURL, err := url.Parse(agent)
		if err != nil {
			return "", ""
		}
		// eg: parsedURL.Path = organizations/buildkite/agents/018a2b90-ba7f-4220-94ca-4903fa0ba410
		// or for clustered agents, parsedURL.Path = organizations/buildkite/clusters/840b09eb-d325-482f-9ff4-0c3abf38560b/queues/fb85c9e4-5531-47a2-90f3-5540dc698811/agents/018c3d27-147b-4faa-94d0-f4c8ce613e5c
		part := strings.Split(parsedURL.Path, "/")
		if len(part) > 4 && part[3] == "agents" {
			org, id = part[2], part[4]
		} else if len(part) > 0 {
			org, id = part[2], part[len(part)-1]
		}
	} else {
		if agentIsSlug {
			part := strings.Split(agent, "/")
			if len(part) >= 2 {
				org, id = part[0], part[1]
			}
		} else {
			org = conf.OrganizationSlug()
			id = agent
		}
	}

	return org, id
}

// watchAgent continuously monitors an agent and displays its status
func watchAgent(ctx context.Context, f *factory.Factory, org, agentID string) error {
	fmt.Printf("Watching agent %s (press Ctrl+C to stop)\n", agentID)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastStatus string

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var agentData buildkite.Agent
			var err error

			agentData, _, err = f.RestAPIClient.Agents.Get(ctx, org, agentID)
			if err != nil {
				fmt.Printf("Error fetching agent: %v\n", err)
				continue
			}

			status := "unknown"
			if agentData.ConnectedState != "" {
				status = agentData.ConnectedState
			}

			// Only print if status changed
			if status != lastStatus {
				timestamp := time.Now().Format("15:04:05")
				fmt.Printf("[%s] Agent %s: %s\n", timestamp, agentID, status)
				lastStatus = status
			}
		}
	}
}
