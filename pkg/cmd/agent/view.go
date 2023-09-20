package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/printer"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAgentView(f *factory.Factory) *cobra.Command {
	var output string
	cmd := cobra.Command{
		Use:   "view",
		Args:  cobra.ExactArgs(1),
		Short: "View an agent",
		Long:  "View an agent by passing in its UUID",
		Example: heredoc.Doc(`
            $ buildkite-agent view 9df48c7e-21d1-4a8e-b862-4b9decb70abd
        `),

		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			output, _ = cmd.Flags().GetString("output")
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
