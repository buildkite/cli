package printer

import (
	"encoding/json"
	"fmt"

	"github.com/buildkite/go-buildkite/v3/buildkite"
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
		return printJSON(agents)
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
