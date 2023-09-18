package agent

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/printer"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	var output string
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",

		Args:  cobra.NoArgs,
		Short: "Lists the agents for the current organization",
		Long: heredoc.Doc(`
            Command to list all agents for the current organization.

            Use the --output flag to change the output format. One of: json|yaml

            Example:

            $bk agent list --output=json
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &buildkite.AgentListOptions{})
			if err != nil {
				return err
			}
			err = printer.PrintOutput(printer.Output(output), agents)
			if err != nil {
				return err
			}
			return err
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format. One of: json|yaml")
	return &cmd
}
