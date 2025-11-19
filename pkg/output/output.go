package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/pflag"
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
	FormatText Format = "text"
)

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

// AddFlags adds format flag to the command flags
func AddFlags(flags *pflag.FlagSet) {
	flags.StringP("output", "o", "json", "Output format. One of: json, yaml, text")
}

// GetFormat gets the format from command flags
func GetFormat(flags *pflag.FlagSet) (Format, error) {
	format, err := flags.GetString("output")
	if err != nil {
		return "", err
	}

	switch Format(format) {
	case FormatJSON, FormatYAML, FormatText:
		return Format(format), nil
	default:
		return "", fmt.Errorf("unsupported output format: %s", format)
	}
}
