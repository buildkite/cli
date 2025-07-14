package cli

import (
	"encoding/json"
	"fmt"
	"os"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"gopkg.in/yaml.v3"
)

// ErrorOutput represents structured error output
type ErrorOutput struct {
	SchemaVersion string `json:"schema_version"`
	Error         Error  `json:"error"`
}

// Error represents the error details
type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Hint        string `json:"hint,omitempty"`
	ExitCode    int    `json:"exit_code"`
	Recoverable bool   `json:"recoverable"`
}

// FormatError formats errors based on output format
func FormatError(err error, outputFormat string, verbose bool) {
	if outputFormat == "json" || outputFormat == "yaml" {
		formatStructuredError(err, outputFormat, verbose)
	} else {
		formatTextError(err, verbose)
	}
}

// formatStructuredError outputs errors in JSON/YAML format
func formatStructuredError(err error, format string, verbose bool) {
	errorOutput := ErrorOutput{
		SchemaVersion: "1",
		Error:         mapErrorToStruct(err, verbose),
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stderr)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(errorOutput)
	case "yaml":
		encoder := yaml.NewEncoder(os.Stderr)
		encoder.SetIndent(2)
		_ = encoder.Encode(errorOutput)
		encoder.Close()
	}
}

// formatTextError outputs errors in human-readable format
func formatTextError(err error, verbose bool) {
	if verbose {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// mapErrorToStruct converts Go errors to structured Error format
func mapErrorToStruct(err error, verbose bool) Error {
	errorStruct := Error{
		Message:  err.Error(),
		ExitCode: bkErrors.GetExitCodeForError(err),
	}

	// Map specific error types to codes and hints
	switch {
	case isAuthError(err):
		errorStruct.Code = "AUTH_REQUIRED"
		errorStruct.Hint = "Run 'bk configure' to set up authentication"
		errorStruct.Recoverable = true
	case isNotFoundError(err):
		errorStruct.Code = "RESOURCE_NOT_FOUND"
		errorStruct.Hint = "Verify the resource exists and you have access to it"
		errorStruct.Recoverable = true
	case isNetworkError(err):
		errorStruct.Code = "NETWORK_ERROR"
		errorStruct.Hint = "Check your internet connection and try again"
		errorStruct.Recoverable = true
	case isConfigError(err):
		errorStruct.Code = "CONFIG_ERROR"
		errorStruct.Hint = "Check your configuration with 'bk configure'"
		errorStruct.Recoverable = true
	case isValidationError(err):
		errorStruct.Code = "VALIDATION_ERROR"
		errorStruct.Hint = "Check the command arguments and try again"
		errorStruct.Recoverable = true
	default:
		errorStruct.Code = "GENERAL_ERROR"
		errorStruct.Recoverable = false
	}

	return errorStruct
}

// Helper functions to identify error types
func isAuthError(err error) bool {
	// Check if error is related to authentication
	msg := err.Error()
	return contains(msg, "authentication") ||
		contains(msg, "unauthorized") ||
		contains(msg, "token") ||
		contains(msg, "401")
}

func isNotFoundError(err error) bool {
	// Check if error is related to resource not found
	msg := err.Error()
	return contains(msg, "not found") ||
		contains(msg, "404") ||
		contains(msg, "does not exist")
}

func isNetworkError(err error) bool {
	// Check if error is related to network issues
	msg := err.Error()
	return contains(msg, "connection") ||
		contains(msg, "network") ||
		contains(msg, "timeout") ||
		contains(msg, "dial")
}

func isConfigError(err error) bool {
	// Check if error is related to configuration
	msg := err.Error()
	return contains(msg, "config") ||
		contains(msg, "configuration") ||
		contains(msg, "organization not set")
}

func isValidationError(err error) bool {
	// Check if error is related to validation
	msg := err.Error()
	return contains(msg, "invalid") ||
		contains(msg, "required") ||
		contains(msg, "validation") ||
		contains(msg, "format")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
