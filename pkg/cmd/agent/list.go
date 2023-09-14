package agent

import (
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	t := table.NewWriter()
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "Lists the agents for the current organization",
		Long: heredoc.Doc(`
            Command to list all agents for the current organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &buildkite.AgentListOptions{})
			if err != nil {
				return err
			}
			t.AppendHeader(table.Row{"#", "Name", "ID", "State"})
			for i, agent := range agents {
				t.AppendRow(table.Row{i + 1, *agent.Name, *agent.ID, *agent.ConnectedState})
			}
			fmt.Println(t.Render())
			return err
		},
	}

	return &cmd
}
