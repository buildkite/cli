package output

import (
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Format represents the output format type
type Format string

const (
	// FormatJSON outputs in JSON format
	FormatJSON Format = "json"
	// FormatYAML outputs in YAML format
	FormatYAML Format = "yaml"
	// FormatText outputs in plain text/default format
	FormatText    Format = "text"
	DefaultFormat Format = FormatJSON
)

// ResolveFormat determines the output format to use.
// Priority: flagValue (if set) > configValue > DefaultFormat
func ResolveFormat(flagValue, configValue string) Format {
	if flagValue != "" {
		return Format(flagValue)
	}
	if configValue != "" {
		return Format(configValue)
	}
	return DefaultFormat
}

// Formatter is an interface that types must implement to support formatted output
type Formatter interface {
	// TextOutput returns the plain text representation
	TextOutput() string
}

// Write outputs the given value in the specified format to the writer
func Write(w io.Writer, v interface{}, format Format) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, v)
	case FormatYAML:
		return writeYAML(w, v)
	case FormatText:
		return writeText(w, v)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func writeJSON(w io.Writer, v interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func writeYAML(w io.Writer, v interface{}) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	return encoder.Encode(v)
}

func writeText(w io.Writer, v interface{}) error {
	if f, ok := v.(Formatter); ok {
		_, err := fmt.Fprintln(w, f.TextOutput())
		return err
	}
	// Fallback to default string representation
	_, err := fmt.Fprintln(w, v)
	return err
}
