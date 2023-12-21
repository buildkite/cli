package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "stop <agent> [--force]",
		Args:                  cobra.ArbitraryArgs,
		Short:                 "Stop an agent",
		Long: heredoc.Doc(`
			Instruct an agent to stop accepting new build jobs and shut itself down.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.

			If the agent is already stopped the command returns an error.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				// create a bubbletea program to manage the output of this command
				l := io.NewPendingCommand(func() tea.Msg {
					org, agent := parseAgentArg(args[0], f.Config)
					_, err := f.RestAPIClient.Agents.Stop(org, agent, force)
					if err != nil {
						return err
					}
					return io.PendingOutput("Agent stopped")
				}, "Stopping agent")

				p := tea.NewProgram(l)
				_, err := p.Run()

				return err
			} else {
				// Loop over the agents passed through the >1 args
				for _, agent := range(args){
					org, agentParsed := parseAgentArg(agent, f.Config)
					_, err := f.RestAPIClient.Agents.Stop(org, agentParsed, force)
					if err != nil {
						return err
					}
					fmt.Println(fmt.Sprintf("Agent %s stopped", agentParsed))
				}
				return nil
			}
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent. Terminating any jobs in progress")

	return &cmd
}
