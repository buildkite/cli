package agent

import (
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAgentStop(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := cobra.Command{
		Use:   "stop <slug>/<uuid> [--force]",
		Args:  cobra.ExactArgs(1),
		Short: "Stop an agent",
		Long:  "Instruct an agent to stop accepting new build jobs and shut itself down.",
		RunE: func(cmd *cobra.Command, args []string) error {
			part := strings.Split(args[0], "/")
			org, agent := part[0], part[1]

			_, err := f.RestAPIClient.Agents.Stop(org, agent, force)
			// TODO: this can return 422 if agent is already disconnected. should we report that as an error or treat
			// this command declaratively where we want to stop the agent so if its already stopped then we dont care

			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force stop the agent")

	return &cmd
}
