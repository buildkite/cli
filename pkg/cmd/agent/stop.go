package agent

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

type AgentStopOptions struct {
	force bool
	limit int64
	f     *factory.Factory
}

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	options := AgentStopOptions{
		f: f,
	}

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "stop <agent>... [--force]",
		Args:                  cobra.ArbitraryArgs,
		Short:                 "Stop Buildkite agents",
		Long: heredoc.Doc(`
			Instruct one or more agents to stop accepting new build jobs and shut itself down.
			Agents can be supplied as positional arguments or from STDIN, one per line.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.

			The --force flag applies to all agents that are stopped.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunStop(cmd, args, &options)
		},
	}

	cmd.Flags().BoolVar(&options.force, "force", false, "Force stop the agent. Terminating any jobs in progress")
	cmd.Flags().Int64VarP(&options.limit, "limit", "l", 5, "Limit parallel API requests")

	return &cmd
}

func RunStop(cmd *cobra.Command, args []string, opts *AgentStopOptions) error {
	// use a wait group to ensure we exit the program after all agents have finished
	var wg sync.WaitGroup
	// this semaphore is used to limit how many concurrent API requests can be sent
	sem := semaphore.NewWeighted(opts.limit)

	var agents []agent.StoppableAgent
	// this command accepts either input from stdin or positional arguments (not both) in that order
	// so we need to check if stdin has data for us to read and read that, otherwise use positional args and if
	// there are none, then we need to error
	// if stdin has data available, use that
	if bk_io.HasDataAvailable(cmd.InOrStdin()) {
		scanner := bufio.NewScanner(cmd.InOrStdin())
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			id := scanner.Text()
			if strings.TrimSpace(id) != "" {
				wg.Add(1)
				agents = append(agents, agent.NewStoppableAgent(id, stopper(cmd.Context(), id, opts.f, opts.force, sem, &wg)))
			}
		}

		if scanner.Err() != nil {
			return scanner.Err()
		}
	} else if len(args) > 0 {
		for _, id := range args {
			if strings.TrimSpace(id) != "" {
				wg.Add(1)
				agents = append(agents, agent.NewStoppableAgent(id, stopper(cmd.Context(), id, opts.f, opts.force, sem, &wg)))
			}
		}
	} else {
		return errors.New("must supply agents to stop")
	}

	bulkAgent := agent.BulkAgent{
		Agents: agents,
	}

	p := tea.NewProgram(bulkAgent, tea.WithOutput(cmd.OutOrStdout()))

	// send a quit message after all agents have stopped
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
