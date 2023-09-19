package printer

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

type Output string

const (
	JSON Output = "json"
	YAML Output = "yaml"
)

func PrintOutput(output Output, p any) (string, error) {
	switch output {
	case JSON:
		return printJSON(p)
	case YAML:
		return printYAML(p)
	default:
		return printJSON(p)
	}
}

func printJSON(p any) (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printYAML(p any) (string, error) {
	data, err := yaml.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
