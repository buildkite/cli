package agent

import (
	"encoding/json"
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Output string

const (
	JSON Output = "json"
	YAML Output = "yaml"
)

func printOutput(output Output, agents []buildkite.Agent) error {
	switch output {
	case JSON:
		return printJSON(agents)
	case YAML:
		return printYAML(agents)
	default:
		return printTable(agents)
	}
}

func printJSON(agents []buildkite.Agent) error {
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printYAML(agents []buildkite.Agent) error {
	data, err := yaml.Marshal(agents)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printTable(agents []buildkite.Agent) error {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"#", "Name", "ID", "State"})
	for i, agent := range agents {
		t.AppendRow(table.Row{i + 1, *agent.Name, *agent.ID, *agent.ConnectedState})
	}
	fmt.Println(t.Render())
	return nil
}

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
			err = printOutput(Output(output), agents)
			if err != nil {
				return err
			}
			return err
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format. One of: json|yaml")
	return &cmd
}
