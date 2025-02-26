package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"github.com/xeipuuv/gojsonschema"
)

const (
	schemaURL = "https://raw.githubusercontent.com/buildkite/pipeline-schema/main/schema.json"
)

func NewCmdPipelineValidate(f *factory.Factory) *cobra.Command {
	var filePaths []string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "validate [flags]",
		Short:                 "Validate a pipeline YAML file",
		Args:                  cobra.NoArgs,
		Long: heredoc.Doc(`
			Validate a pipeline YAML file against the Buildkite pipeline schema.

			By default, this command looks for a file at .buildkite/pipeline.yaml or .buildkite/pipeline.yml
			in the current directory. You can specify different files using the --file flag.
		`),
		Example: heredoc.Doc(`
			# Validate the default pipeline file
			$ bk pipeline validate

			# Validate a specific pipeline file
			$ bk pipeline validate --file path/to/pipeline.yaml

			# Validate multiple pipeline files
			$ bk pipeline validate --file path/to/pipeline1.yaml --file path/to/pipeline2.yaml
		`),
		// Skip API token validation as pipeline validation is a local operation
		// and doesn't require API access
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no file paths provided, find the default
			if len(filePaths) == 0 {
				defaultPath, err := findPipelineFile()
				if err != nil {
					return err
				}
				filePaths = []string{defaultPath}
			}

			// Track if any validation failed
			hasErrors := false

			// Validate each file
			for _, filePath := range filePaths {
				err := validatePipeline(cmd.OutOrStdout(), filePath)
				if err != nil {
					hasErrors = true
					// Continue validating other files even if one fails
				}
			}

			if hasErrors {
				return fmt.Errorf("pipeline validation failed")
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&filePaths, "file", "f", []string{}, "Path to the pipeline YAML file(s) to validate")

	return &cmd
}

// findPipelineFile attempts to locate a pipeline file in the default locations
func findPipelineFile() (string, error) {
	// Check for pipeline.yaml or pipeline.yml in .buildkite directory
	paths := []string{
		filepath.Join(".buildkite", "pipeline.yaml"),
		filepath.Join(".buildkite", "pipeline.yml"),
	}

	for _, path := range paths {
		if fileExists(path) {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find pipeline file in default locations (.buildkite/pipeline.yaml or .buildkite/pipeline.yml), please specify one with --file")
}

// fileExists checks if a file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// validatePipeline validates the given pipeline file against the schema
func validatePipeline(w io.Writer, filePath string) error {
	// Read the pipeline file
	pipelineData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading pipeline file: %w", err)
	}

	// Convert YAML to JSON for validation
	jsonData, err := yaml.YAMLToJSON(pipelineData)
	if err != nil {
		return fmt.Errorf("error converting YAML to JSON: %w", err)
	}

	// Load the schema and document
	schemaLoader := gojsonschema.NewReferenceLoader(schemaURL)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	// Validate against the schema
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("error validating pipeline: %w", err)
	}

	if result.Valid() {
		fmt.Fprintf(w, "✅ Pipeline file is valid: %s\n", filePath)
		return nil
	}

	// Return validation errors
	fmt.Fprintf(w, "❌ Pipeline file is invalid: %s\n\n", filePath)
	for _, err := range result.Errors() {
		// Format the error message for better readability
		message := formatValidationError(err)
		fmt.Fprintf(w, "- %s\n", message)
	}

	return fmt.Errorf("pipeline validation failed")
}

// formatValidationError formats a validation error for better readability
func formatValidationError(err gojsonschema.ResultError) string {
	// Clean up the context to make it more readable
	context := err.Context().String()
	field := err.Field()

	// If the context is just the root, simplify it
	if context == "(root)" {
		context = ""
	}

	// For array items, make the error message more readable
	if strings.Contains(field, "[") && strings.Contains(field, "]") {
		parts := strings.Split(field, ".")
		for i, part := range parts {
			if strings.Contains(part, "[") {
				index := strings.TrimRight(strings.TrimLeft(part, "["), "]")
				name := strings.Split(part, "[")[0]
				parts[i] = fmt.Sprintf("%s item #%s", name, index)
			}
		}
		field = strings.Join(parts, " > ")
	} else if field != "" {
		field = strings.ReplaceAll(field, ".", " > ")
	}

	message := err.Description()
	if field != "" {
		message = fmt.Sprintf("%s: %s", field, message)
	}

	// Include details about what was received vs what was expected if available
	details := err.Details()
	if val, ok := details["field"]; ok && val != field {
		message += fmt.Sprintf(" (field: %v)", val)
	}
	if val, ok := details["expected"]; ok {
		message += fmt.Sprintf(" (expected: %v)", val)
	}
	if val, ok := details["actual"]; ok {
		message += fmt.Sprintf(" (got: %v)", val)
	}

	return message
}
