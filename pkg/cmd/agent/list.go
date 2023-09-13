package agent

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/cobra"
    "fmt"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "Lists the agents for the current organization",
		Long: heredoc.Doc(`
            Command to list all agents for the current organization.


            Only running agents are listed by default. To list all agents, use the
            --all flag.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
            agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &buildkite.AgentListOptions{})

            for i, agent := range agents {
                fmt.Printf("%d: %s. ID: %s. State: %s\n", i+1, *agent.Name, *agent.ID, *agent.ConnectedState)
            }
			return err
		},
	}

	return &cmd
}
