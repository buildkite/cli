package agent

import (
	"errors"
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
			switch agents := len(args); {
			case agents == 0:
				// No agents slug/UUID passed in, return an error
				return errors.New("Please specify at least one agent to stop.")
			case agents == 1:
				return stopAgent(args[0], f, force)
			case agents >= 2:
				// Construct an errors variable to append agent stop errors
				var errors error
				for _, agent := range args {
					_ = stopAgent(agent, f, force) // Stop singular agent
				}
				return errors
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent. Terminating any jobs in progress")

	return &cmd
}

func stopAgent(agent string, f *factory.Factory, force bool) error {
	l := io.NewPendingCommand(func() tea.Msg {
		org, agent := parseAgentArg(agent, f.Config)
		_, err := f.RestAPIClient.Agents.Stop(org, agent, force)
		if err != nil {
			return err
		}
		return io.PendingOutput(fmt.Sprintf("Stopped agent %s", agent))
	}, fmt.Sprintf("Stopping agent %s", agent))

	p := tea.NewProgram(l)
	_, err := p.Run()

	return err
}