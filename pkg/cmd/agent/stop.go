package agent

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	var force bool

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
			model := agent.NewStoppableAgent(args[0], func() agent.StatusUpdate {
				org, agentID := parseAgentArg(args[0], f.Config)

				return agent.StatusUpdate{
					Status: agent.Stopping,
					Cmd: func() tea.Msg {
						_, err := f.RestAPIClient.Agents.Stop(org, agentID, force)
						if err != nil {
							return agent.StatusUpdate{
								Err: err,
								Cmd: tea.Quit,
							}
						}
						return agent.StatusUpdate{
							Status: agent.Succeeded,
							Cmd:    tea.Quit,
						}
					},
				}
			})
			p := tea.NewProgram(model)
			_, err := p.Run()
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent. Terminating any jobs in progress")

	return &cmd
}
