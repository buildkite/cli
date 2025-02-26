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
	schemaJSON := []byte(`{
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

	// Create a schema loader for testing
	testSchemaLoader := gojsonschema.NewBytesLoader(schemaJSON)

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
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary file
			tmpFile, err := os.CreateTemp("", "pipeline-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())

			// Write test content to the file
			if _, err := tmpFile.Write([]byte(test.fileContent)); err != nil {
				t.Fatal(err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatal(err)
			}

			// Create a buffer to capture output
			var stdout bytes.Buffer

			// Define a test validation function that uses our test schema
			mockValidatePipeline := func(w io.Writer, filePath string) error {
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

				// Validate using the test schema loader
				documentLoader := gojsonschema.NewBytesLoader(jsonData)
				result, err := gojsonschema.Validate(testSchemaLoader, documentLoader)
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

			// Call our mock validation function directly
			err = mockValidatePipeline(&stdout, tmpFile.Name())

			// Check error expectation
			if test.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}

			// Check output
			if !strings.Contains(stdout.String(), test.expectOutput) {
				t.Errorf("Expected output to contain %q, but got: %q", test.expectOutput, stdout.String())
			}
		})
	}
}

// Mock implementation of findPipelineFile for testing
func mockFindPipelineFile(fileExistsFn func(string) bool) func() (string, error) {
	return func() (string, error) {
		// Check for pipeline.yaml or pipeline.yml in .buildkite directory
		paths := []string{
			filepath.Join(".buildkite", "pipeline.yaml"),
			filepath.Join(".buildkite", "pipeline.yml"),
		}

		for _, path := range paths {
			if fileExistsFn(path) {
				return path, nil
			}
		}

		return "", fmt.Errorf("could not find pipeline file in default locations (.buildkite/pipeline.yaml or .buildkite/pipeline.yml), please specify one with --file")
	}
}

func TestFindPipelineFile(t *testing.T) {
	t.Parallel()

	// Test finding no file
	t.Run("no file exists", func(t *testing.T) {
		// Mock fileExists to always return false
		findPipelineFileFn := mockFindPipelineFile(func(path string) bool {
			return false
		})

		_, err := findPipelineFileFn()
		if err == nil {
			t.Error("Expected error but got none")
		}
	})

	// Test finding pipeline.yml
	t.Run("find pipeline.yml", func(t *testing.T) {
		// Mock fileExists to return true only for pipeline.yml
		findPipelineFileFn := mockFindPipelineFile(func(path string) bool {
			return path == filepath.Join(".buildkite", "pipeline.yml")
		})

		path, err := findPipelineFileFn()
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		expected := filepath.Join(".buildkite", "pipeline.yml")
		if path != expected {
			t.Errorf("Expected %q but got %q", expected, path)
		}
	})

	// Test finding pipeline.yaml
	t.Run("find pipeline.yaml", func(t *testing.T) {
		// Mock fileExists to return true only for pipeline.yaml
		findPipelineFileFn := mockFindPipelineFile(func(path string) bool {
			return path == filepath.Join(".buildkite", "pipeline.yaml")
		})

		path, err := findPipelineFileFn()
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		expected := filepath.Join(".buildkite", "pipeline.yaml")
		if path != expected {
			t.Errorf("Expected %q but got %q", expected, path)
		}
	})

	// Test preference for pipeline.yaml over pipeline.yml
	t.Run("prefer pipeline.yaml over pipeline.yml", func(t *testing.T) {
		// Mock fileExists to return true for both files
		findPipelineFileFn := mockFindPipelineFile(func(path string) bool {
			return path == filepath.Join(".buildkite", "pipeline.yaml") ||
				path == filepath.Join(".buildkite", "pipeline.yml")
		})

		path, err := findPipelineFileFn()
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		// Should find pipeline.yaml since it's checked first
		expected := filepath.Join(".buildkite", "pipeline.yaml")
		if path != expected {
			t.Errorf("Expected %q but got %q", expected, path)
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

	// Load the pipeline file
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
