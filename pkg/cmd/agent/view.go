package agent

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/printer"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAgentView(f *factory.Factory) *cobra.Command {
	var id string
	var output string
	cmd := cobra.Command{
		Use:   "view",
		Args:  cobra.NoArgs,
		Short: "View an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ = cmd.Flags().GetString("id")
			output, _ = cmd.Flags().GetString("output")
			if id == "" {
				return fmt.Errorf("id is required")
			}
			if output == "" {
				output = "json"
			}
			err := validation.ValidateUUID(id)
			if err != nil {
				return err
			}
			agent, _, err := f.RestAPIClient.Agents.Get(f.Config.Organization, id)
			if err != nil {
				return err
			}
			data, err := printer.PrintOutput(printer.Output(output), agent)
			if err != nil {
				return err
			}
			fmt.Println(data)
			return nil
		},
	}

	cmd.Flags().String("id", "", "The ID of the agent")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format. One of: json|yaml")
	return &cmd
}
