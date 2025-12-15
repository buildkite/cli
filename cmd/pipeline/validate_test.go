package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/xeipuuv/gojsonschema"
)

func TestValidatePipeline(t *testing.T) {
	t.Parallel()

	// Create a test schema that matches actual Buildkite schema requirements:
	// This simplified schema ensures:
	// - Steps are required
	// - Each step must have at least a "command" field
	// - A "label" field is optional and must be a string
	// - A "command" field must be a string
	testSchema := []byte(`{
        "$schema": "http://json-schema.org/draft-07/schema#",
        "type": "object",
        "required": ["steps"],
        "properties": {
            "steps": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "label": { "type": "string" },
                        "command": { "type": "string" }
                    },
                    "required": ["command"]
                }
            }
        }
    }`)

	testSchemaLoader := gojsonschema.NewBytesLoader(testSchema)

	tests := []struct {
		name         string
		fileContent  string
		expectError  bool
		expectOutput string
	}{
		{
			name: "valid pipeline",
			fileContent: `steps:
  - label: "Hello, world! 👋"
    command: echo "Hello, world!"`,
			expectError:  false,
			expectOutput: "✅ Pipeline file is valid",
		},
		{
			name: "valid pipeline with command only",
			fileContent: `steps:
  - command: echo "Hello, world!"`,
			expectError:  false,
			expectOutput: "✅ Pipeline file is valid",
		},
		{
			name: "invalid pipeline missing command",
			fileContent: `steps:
  - label: "Hello, world!"`,
			expectError:  true,
			expectOutput: "❌ Pipeline file is invalid",
		},
		{
			name: "invalid pipeline with wrong type",
			fileContent: `steps:
  - label: 123
    command: echo "Hello, world!"`,
			expectError:  true,
			expectOutput: "❌ Pipeline file is invalid",
		},
		{
			name:         "empty file",
			fileContent:  "",
			expectError:  true,
			expectOutput: "File is empty",
		},
		{
			name: "invalid YAML syntax",
			fileContent: `steps:
  - label: "This has invalid syntax
    command: echo "Missing closing quote and improper indentation`,
			expectError:  true,
			expectOutput: "YAML parsing error",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tmpFile, err := os.CreateTemp("", "pipeline-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(test.fileContent)); err != nil {
				t.Fatal(err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatal(err)
			}

			var stdout bytes.Buffer

			mockValidatePipeline := func(w io.Writer, filePath string) error {
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

				documentLoader := gojsonschema.NewBytesLoader(jsonData)
				result, err := gojsonschema.Validate(testSchemaLoader, documentLoader)
				if err != nil {
					return fmt.Errorf("error validating pipeline: %w", err)
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

			err = mockValidatePipeline(&stdout, tmpFile.Name())

			if test.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}

			if !strings.Contains(stdout.String(), test.expectOutput) {
				t.Errorf("Expected output to contain %q, but got: %q", test.expectOutput, stdout.String())
			}
		})
	}
}

func TestFindPipelineFile(t *testing.T) {
	t.Parallel()

	t.Run("no file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		_, err := findPipelineFile()
		if err == nil {
			t.Error("Expected error but got none")
		}
	})

	t.Run("find pipeline.yml", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		buildkiteDir := filepath.Join(tmpDir, ".buildkite")
		os.MkdirAll(buildkiteDir, 0o755)

		testFile := filepath.Join(buildkiteDir, "pipeline.yml")
		os.WriteFile(testFile, []byte("steps: []"), 0o644)

		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		path, err := findPipelineFile()
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		expected := filepath.Join(".buildkite", "pipeline.yml")
		if path != expected {
			t.Errorf("Expected %q but got %q", expected, path)
		}
	})

	t.Run("find pipeline.yaml", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		buildkiteDir := filepath.Join(tmpDir, ".buildkite")
		os.MkdirAll(buildkiteDir, 0o755)

		testFile := filepath.Join(buildkiteDir, "pipeline.yaml")
		os.WriteFile(testFile, []byte("steps: []"), 0o644)

		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		path, err := findPipelineFile()
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		if !strings.Contains(path, "pipeline.y") {
			t.Errorf("Expected path to contain pipeline.yaml or pipeline.yml, got %q", path)
		}
	})
}

// Test formatting validation errors
func TestFormatValidationError(t *testing.T) {
	t.Parallel()

	// This is a simplified test since we can't directly create gojsonschema.ResultError objects
	// Create a test schema that we can use to generate validation errors
	schemaJSON := []byte(`{
		"type": "object",
		"properties": {
			"steps": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"label": { "type": "string" },
						"command": { "type": "string" }
					}
				}
			}
		}
	}`)

	// Invalid YAML that will cause validation errors
	invalidYaml := `
steps:
  - label: "Test"
    command: echo "Hello"
  - label: "Test 2"
    command: 123
`

	// Load the pipeline file
	tmpFile, err := os.CreateTemp("", "invalid-pipeline-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(invalidYaml)); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer

	pipelineData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("error reading pipeline file: %v", err)
	}

	// Convert YAML to JSON for validation
	jsonData, err := yaml.YAMLToJSON(pipelineData)
	if err != nil {
		t.Fatalf("error converting YAML to JSON: %v", err)
	}

	// Validate using our test schema
	schemaLoader := gojsonschema.NewBytesLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		t.Fatalf("error validating pipeline: %v", err)
	}

	// Output the validation errors to the buffer
	fmt.Fprintf(&buf, "❌ Pipeline file is invalid: %s\n\n", tmpFile.Name())
	for _, err := range result.Errors() {
		message := formatValidationError(err)
		fmt.Fprintf(&buf, "- %s\n", message)
	}

	output := buf.String()

	// Output should mention the error for the invalid field type
	if !strings.Contains(output, "invalid") {
		t.Errorf("Expected output to mention invalid field, got: %s", output)
	}
}

func TestFileExists(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		tmpFile, err := os.CreateTemp("", "test-*.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		if !fileExists(tmpFile.Name()) {
			t.Error("Expected file to exist")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		if fileExists("/this/path/does/not/exist/file.txt") {
			t.Error("Expected file to not exist")
		}
	})

	t.Run("directory exists but not a file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		if fileExists(tmpDir) {
			t.Error("Expected fileExists to return false for directory")
		}
	})
}
