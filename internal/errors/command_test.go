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
		t.Parallel()

		var buf bytes.Buffer
		cmd := &cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, args []string) error {
				return NewValidationError(nil, "Invalid input")
			},
		}

		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		cmd.SetErr(&buf)

		handler := NewCommandErrorHandler()
		handler.HandleCommandError(cmd, NewValidationError(nil, "Invalid input"))

		output := stripANSI(buf.String())
		if !strings.Contains(output, "Validation Error:") {
			t.Errorf("Expected output to contain error type, got: %q", output)
		}
		if !strings.Contains(output, "Invalid input") {
			t.Errorf("Expected output to contain error message, got: %q", output)
		}
		if !strings.Contains(output, "test") {
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
				originalError:        NewResourceNotFoundError(nil, "Resource not found"),
				expectSuggestionHelp: true,
				expectCommandContext: false,
			},
			{
				name:                 "validation error",
				originalError:        NewValidationError(nil, "Invalid input"),
				expectSuggestionHelp: false,
				expectCommandContext: true,
			},
			{
				name:                 "other error",
				originalError:        fmt.Errorf("generic error"),
				expectSuggestionHelp: false,
				expectCommandContext: false,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Create a command with a wrapped RunE function
				cmd := &cobra.Command{
					Use: "test",
					RunE: WrapRunE(func(cmd *cobra.Command, args []string) error {
						return tc.originalError
					}),
				}

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
