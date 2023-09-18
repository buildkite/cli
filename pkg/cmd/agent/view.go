package agent

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/printer"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAgentView(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "view",
		Args:  cobra.NoArgs,
		Short: "View an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := cmd.Flags().GetString("id")
			if id == "" {
				return fmt.Errorf("id is required")
			}
			validation.ValidateUUID(id)
			agent, _, err := f.RestAPIClient.Agents.Get(f.Config.Organization, id)
			if err != nil {
				return err
			}
			printer.PrintOutput("", agent)
			return nil
		},
	}

	cmd.Flags().String("id", "", "The ID of the agent")
	return &cmd
}
