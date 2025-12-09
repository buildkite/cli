package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/ghodss/yaml"
	"github.com/xeipuuv/gojsonschema"
)

const schemaURL = "https://raw.githubusercontent.com/buildkite/pipeline-schema/main/schema.json"

var fallbackSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"properties": {
		"steps": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"label": { "type": "string" },
					"command": { "type": "string" },
					"plugins": { "type": "object" },
					"agents": { "type": "object" },
					"env": { "type": "object" },
					"branches": { "type": ["string", "object"] },
					"if": { "type": "string" },
					"depends_on": { "type": ["string", "array"] }
				}
			}
		},
		"env": { "type": "object" },
		"agents": { "type": "object" }
	},
	"required": ["steps"]
}`)

type ValidateCmd struct {
	File []string `help:"Path to the pipeline YAML file(s) to validate" short:"f"`
}

func (c *ValidateCmd) Help() string {
	return `Validate a pipeline YAML file against the Buildkite pipeline schema.

By default, this command looks for a file at .buildkite/pipeline.yaml or .buildkite/pipeline.yml
in the current directory. You can specify different files using the --file flag.

Note: This command does not require an API token since validation is done locally.

Examples:
  # Validate the default pipeline file
  $ bk pipeline validate

  # Validate a specific pipeline file
  $ bk pipeline validate --file path/to/pipeline.yaml

  # Validate multiple pipeline files
  $ bk pipeline validate --file path/to/pipeline1.yaml --file path/to/pipeline2.yaml
`
}

func (c *ValidateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	filePaths := c.File
	if len(filePaths) == 0 {
		defaultPath, err := findPipelineFile()
		if err != nil {
			return err
		}
		filePaths = []string{defaultPath}
	}

	var validationErrors []string
	fileCount := len(filePaths)

	fmt.Printf("Validating %d pipeline file(s)...\n\n", fileCount)

	for _, filePath := range filePaths {
		err := validatePipeline(os.Stdout, filePath)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %v", filePath, err))
		}
	}

	if len(validationErrors) > 0 {
		errorCount := len(validationErrors)
		fmt.Printf("\n%d of %d file(s) failed validation.\n", errorCount, fileCount)
		return fmt.Errorf("pipeline validation failed")
	}

	fmt.Println("\nAll pipeline files passed validation successfully!")
	return nil
}

func findPipelineFile() (string, error) {
	paths := []string{
		"buildkite.yml",
		"buildkite.yaml",
		"buildkite.json",
		filepath.Join(".buildkite", "pipeline.yml"),
		filepath.Join(".buildkite", "pipeline.yaml"),
		filepath.Join(".buildkite", "pipeline.json"),
		filepath.Join("buildkite", "pipeline.yml"),
		filepath.Join("buildkite", "pipeline.yaml"),
		filepath.Join("buildkite", "pipeline.json"),
	}

	for _, path := range paths {
		if fileExists(path) {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find pipeline file in default locations. Please specify a file with --file or create one in a standard location:\n" +
		"  • .buildkite/pipeline.yml\n" +
		"  • .buildkite/pipeline.yaml\n" +
		"  • buildkite.yml\n" +
		"  • buildkite.yaml")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func validatePipeline(w io.Writer, filePath string) error {
	pipelineData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading pipeline file: %w", err)
	}

	if len(strings.TrimSpace(string(pipelineData))) == 0 {
		fmt.Fprintf(w, "❌ Pipeline file is invalid: %s\n\n", filePath)
		fmt.Fprintf(w, "- File is empty\n")
		return fmt.Errorf("empty pipeline file")
	}

	jsonData, err := yaml.YAMLToJSON(pipelineData)
	if err != nil {
		fmt.Fprintf(w, "❌ Pipeline file is invalid: %s\n\n", filePath)
		fmt.Fprintf(w, "- YAML parsing error: %s\n", err.Error())
		fmt.Fprintf(w, "  Hint: Check for syntax errors like improper indentation, missing quotes, or invalid characters.\n")
		return fmt.Errorf("invalid YAML format: %w", err)
	}

	schemaLoader := gojsonschema.NewReferenceLoader(schemaURL)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		fmt.Fprintf(w, "⚠️  Warning: Could not access online pipeline schema: %s\n", err.Error())
		fmt.Fprintf(w, "   Using simplified fallback schema for basic validation.\n\n")

		fallbackLoader := gojsonschema.NewBytesLoader(fallbackSchema)
		result, err = gojsonschema.Validate(fallbackLoader, documentLoader)
		if err != nil {
			fmt.Fprintf(w, "❌ Pipeline file is invalid: %s\n\n", filePath)
			fmt.Fprintf(w, "- Schema validation error: %s\n", err.Error())
			return fmt.Errorf("schema validation error: %w", err)
		}
	}

	if result.Valid() {
		fmt.Fprintf(w, "✅ Pipeline file is valid: %s\n", filePath)
		return nil
	}

	fmt.Fprintf(w, "❌ Pipeline file is invalid: %s\n\n", filePath)
	for _, err := range result.Errors() {
		message := formatValidationError(err)
		fmt.Fprintf(w, "- %s\n", message)
	}

	return fmt.Errorf("pipeline validation failed")
}

func formatValidationError(err gojsonschema.ResultError) string {
	field := err.Field()

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

	details := err.Details()

	var contextParts []string
	if val, ok := details["field"]; ok && val != field {
		contextParts = append(contextParts, fmt.Sprintf("field: %v", val))
	}
	if val, ok := details["expected"]; ok {
		contextParts = append(contextParts, fmt.Sprintf("expected: %v", val))
	}
	if val, ok := details["actual"]; ok {
		contextParts = append(contextParts, fmt.Sprintf("got: %v", val))
	}

	if len(contextParts) > 0 {
		message += fmt.Sprintf(" (%s)", strings.Join(contextParts, ", "))
	}

	switch err.Type() {
	case "required":
		message += "\n    Hint: This field is required but was not found in your pipeline."
	case "type_error":
		message += "\n    Hint: Check that you're using the correct data type for this field."
	case "enum":
		message += "\n    Hint: The value must be one of the allowed options."
	case "const":
		message += "\n    Hint: This field must have the specific required value."
	case "array_no_items":
		message += "\n    Hint: This array cannot be empty."
	}

	return message
}
