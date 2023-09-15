package printer

import (
	"encoding/json"
	"fmt"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/jedib0t/go-pretty/v6/table"
	"gopkg.in/yaml.v3"
)

type Output string

const (
	JSON Output = "json"
	YAML Output = "yaml"
)

// It would be great to make this either generic or accepting of things that
// implement a suitable interface.

func PrintOutput(output Output, agents []buildkite.Agent) error {
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
