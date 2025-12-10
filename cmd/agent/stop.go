package agent

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"golang.org/x/sync/semaphore"
)

type StopCmd struct {
	Agents []string `arg:"" optional:"" help:"Agent IDs to stop"`
	Force  bool     `help:"Force stop the agent. Terminating any jobs in progress"`
	Limit  int64    `help:"Limit parallel API requests" short:"l" default:"5"`
}

func (c *StopCmd) Help() string {
	return `Instruct one or more agents to stop accepting new build jobs and shut itself down.
Agents can be supplied as positional arguments or from STDIN, one per line.

If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
is omitted, it uses the currently selected organization.

The --force flag applies to all agents that are stopped.

Examples:
  # Stop a single agent
  $ bk agent stop 0198d108-a532-4a62-9bd7-b2e744bf5c45

  # Stop multiple agents
  $ bk agent stop agent-1 agent-2 agent-3

  # Force stop an agent
  $ bk agent stop 0198d108-a532-4a62-9bd7-b2e744bf5c45 --force

  # Stop agents from STDIN
  $ cat agent-ids.txt | bk agent stop`
}

func (c *StopCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
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

	// use a wait group to ensure we exit the program after all agents have finished
	var wg sync.WaitGroup
	// this semaphore is used to limit how many concurrent API requests can be sent
	sem := semaphore.NewWeighted(c.Limit)

	var agents []agent.StoppableAgent
	// this command accepts either input from stdin or positional arguments (not both) in that order
	// so we need to check if stdin has data for us to read and read that, otherwise use positional args and if
	// there are none, then we need to error
	// if stdin has data available, use that
	if bkIO.HasDataAvailable(os.Stdin) {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			id := scanner.Text()
			if strings.TrimSpace(id) != "" {
				wg.Add(1)
				agents = append(agents, agent.NewStoppableAgent(id, stopper(ctx, id, f, c.Force, sem, &wg), f.Quiet))
			}
		}

		if scanner.Err() != nil {
			return scanner.Err()
		}
	} else if len(c.Agents) > 0 {
		for _, id := range c.Agents {
			if strings.TrimSpace(id) != "" {
				wg.Add(1)
				agents = append(agents, agent.NewStoppableAgent(id, stopper(ctx, id, f, c.Force, sem, &wg), f.Quiet))
			}
		}
	} else {
		return errors.New("must supply agents to stop")
	}

	bulkAgent := agent.BulkAgent{
		Agents: agents,
	}

	programOpts := []tea.ProgramOption{tea.WithOutput(os.Stdout)}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		programOpts = append(programOpts, tea.WithInput(nil))
	}
	p := tea.NewProgram(bulkAgent, programOpts...)

	// send a quit message after all agents have stopped
	go func() {
		wg.Wait()
		p.Send(tea.Quit())
	}()

	_, err = p.Run()
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if agent.Errored() {
			return errors.New("at least one agent failed to stop")
		}
	}
	return nil
}

// here we want to allow each agent to transition through from a waiting state to stopping and ending at
// success/failure. so we need to wrap up multiple tea.Cmds, the first one marking it as "stopping". after
// that, another Cmd is started to make the API request to stop it. After that request we return a status to
// indicate success/failure
// the sync.WaitGroup also needs to be marked as done so we can stop the entire application after all agents
// are stopped
func stopper(ctx context.Context, id string, f *factory.Factory, force bool, sem *semaphore.Weighted, wg *sync.WaitGroup) agent.StopFn {
	org, agentID := parseAgentArg(id, f.Config)
	return func() agent.StatusUpdate {
		// before attempting to stop the agent, acquire a semaphore lock to limit parallelisation
		_ = sem.Acquire(context.Background(), 1)

		return agent.StatusUpdate{
			ID:     id,
			Status: agent.Stopping,
			// return an new command to actually stop the agent in the api and return the status of that
			Cmd: func() tea.Msg {
				// defer the semaphore and waitgroup release until the whole operation is completed
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
