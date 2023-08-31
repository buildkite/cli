package agent

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "stop <agent> [--force]",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Stop an agent",
		Long: heredoc.Doc(`
			Instruct an agent to stop accepting new build jobs and shut itself down.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.

			If the agent is already stopped the command returns an error.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, agent := parseAgentArg(args[0], f.Config)

			_, err := f.RestAPIClient.Agents.Stop(org, agent, force)

			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent. Terminating any jobs in progress")

	return &cmd
}
