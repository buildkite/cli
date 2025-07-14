package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"gopkg.in/yaml.v3"
)

// Print outputs an object in the format specified by the global --output flag
func Print(obj any, f *factory.Factory) error {
	switch f.Output {
	case "json":
		return printJSON(obj)
	case "yaml":
		return printYAML(obj)
	case "table", "raw":
		// For table/raw, commands should handle their own human-readable formatting
		// This is a fallback that just prints JSON
		return printJSON(obj)
	default:
		return fmt.Errorf("unknown output format %q", f.Output)
	}
}

// printJSON outputs an object as pretty-printed JSON
func printJSON(obj any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(obj)
}

// printYAML outputs an object as YAML
func printYAML(obj any) error {
	encoder := yaml.NewEncoder(os.Stdout)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(obj)
}

// ShouldUseStructuredOutput returns true if the output format is machine-readable
func ShouldUseStructuredOutput(f *factory.Factory) bool {
	return f.Output == "json" || f.Output == "yaml"
}
