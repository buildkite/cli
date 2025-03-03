package errors

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandErrorHandler(t *testing.T) {
	t.Parallel()

	t.Run("getCommandPath builds full command path", func(t *testing.T) {
		t.Parallel()

		// Create a command hierarchy
		childCmd := &cobra.Command{Use: "child"}
		parentCmd := &cobra.Command{Use: "parent"}
		rootCmd := &cobra.Command{Use: "root"}

		// Set up command hierarchy
		parentCmd.AddCommand(childCmd)
		rootCmd.AddCommand(parentCmd)

		path := getCommandPath(childCmd)
		expected := "root parent child"

		if path != expected {
			t.Errorf("Expected command path %q, got %q", expected, path)
		}
	})

	t.Run("HandleCommandError formats error with command context", func(t *testing.T) {
		// Skip parallelism for this test
		// t.Parallel()

		// Create a unique buffer for this test
		var buf bytes.Buffer
		testCmd := &cobra.Command{
			Use: "test-handling-cmd",
		}

		// Set the error output
		testCmd.SetErr(&buf)

		// Silence any cobra output
		testCmd.SilenceUsage = true
		testCmd.SilenceErrors = true

		// Create a specific test error
		testErr := NewValidationError(nil, "Test validation error")

		// Create a handler and handle the error
		handler := NewCommandErrorHandler()
		handler.handler.WithExitFunc(func(int) {}) // Override exit function to prevent test exits
		handler.HandleCommandError(testCmd, testErr)

		// Check output
		output := stripANSI(buf.String())

		// Test for the error type prefix
		if !strings.Contains(output, "Validation Error:") {
			t.Errorf("Expected output to contain error type, got: %q", output)
		}

		// Test for the error message
		if !strings.Contains(output, "Test validation error") {
			t.Errorf("Expected output to contain error message, got: %q", output)
		}

		// Test for the command name
		if !strings.Contains(output, "test-handling-cmd") {
			t.Errorf("Expected output to contain command name, got: %q", output)
		}
	})

	t.Run("WrapRunE adds context to errors", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name                 string
			originalError        error
			expectSuggestionHelp bool
			expectCommandContext bool
		}{
			{
				name:                 "resource not found error",
				originalError:        NewResourceNotFoundError(nil, "Resource not found for test 1"),
				expectSuggestionHelp: true,
				expectCommandContext: false,
			},
			{
				name:                 "validation error",
				originalError:        NewValidationError(nil, "Invalid input for test 2"),
				expectSuggestionHelp: false,
				expectCommandContext: true,
			},
			{
				name:                 "other error",
				originalError:        fmt.Errorf("generic error for test 3"),
				expectSuggestionHelp: false,
				expectCommandContext: false,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Create a command with a wrapped RunE function
				// Use a unique command name per test case to avoid collisions
				cmd := &cobra.Command{
					Use: "test-" + tc.name,
					RunE: WrapRunE(func(cmd *cobra.Command, args []string) error {
						return tc.originalError
					}),
				}

				// Silence usage and errors to prevent output during tests
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true

				// Run the command
				err := cmd.Execute()

				// Check that the error is still returned
				if err == nil {
					t.Fatal("Expected error, got nil")
				}

				// Check for help suggestion
				if tc.expectSuggestionHelp {
					cliErr, ok := err.(*Error)
					if !ok {
						t.Fatal("Expected error to be a *Error")
					}
					foundHelpSuggestion := false
					for _, suggestion := range cliErr.Suggestions {
						if strings.Contains(suggestion, "--help") {
							foundHelpSuggestion = true
							break
						}
					}
					if !foundHelpSuggestion {
						t.Error("Expected help suggestion for not found error")
					}
				}

				// Check for command context
				if tc.expectCommandContext {
					cliErr, ok := err.(*Error)
					if !ok {
						t.Fatal("Expected error to be a *Error")
					}
					if !strings.Contains(cliErr.Details, "When executing") {
						t.Errorf("Expected details to contain command context, got: %q", cliErr.Details)
					}
				}
			})
		}
	})
}
