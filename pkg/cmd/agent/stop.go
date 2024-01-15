package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	var force bool
	var limit int64

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "stop <agent> [--force]",
		Args:                  cobra.MinimumNArgs(1),
		Short:                 "Stop Buildkite agents",
		Long: heredoc.Doc(`
			Instruct one or more agents to stop accepting new build jobs and shut itself down.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.

			The --force flag applies to all agents that are stopped.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// use a wait group to ensure we exit the program after all agents have finished
			var wg sync.WaitGroup
			wg.Add(len(args))
			// this semaphore is used to limit how many concurrent API requests can be sent
			var sem = semaphore.NewWeighted(limit)

			// here we want to allow each agent to transition through from a waiting state to stopping and ending at
			// success/failure. so we need to wrap up multiple tea.Cmds, the first one marking it as "stopping". after
			// that, another Cmd is started to make the API request to stop it. After that request we return a status to
			// indicate success/failure
			// the sync.WaitGroup also needs to be marked as done so we can stop the entire application after all agents
			// are stopped
			var stopFn = func(id string) agent.StopFn {
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
							_, err := f.RestAPIClient.Agents.Stop(org, agentID, force)
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

			agents := make([]agent.StoppableAgent, len(args))
			for i, id := range args {
				agents[i] = agent.NewStoppableAgent(id, stopFn(id))
			}
			bulkAgent := agent.BulkAgent{
				Agents: agents,
			}

			p := tea.NewProgram(bulkAgent)

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
					return errors.New("At least one agent failed to stop")
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent. Terminating any jobs in progress")
	cmd.Flags().Int64VarP(&limit, "limit", "l", 5, "Limit parallel API requests")

	return &cmd
}
