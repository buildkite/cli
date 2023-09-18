package printer

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Output string

const (
	JSON Output = "json"
	YAML Output = "yaml"
)

func PrintOutput(output Output, p any ) error {
	switch output {
	case JSON:
		return printJSON(p)
	case YAML:
		return printYAML(p)
	default:
		return printJSON(p)
	}
}

func printJSON(p any) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printYAML(p any) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
