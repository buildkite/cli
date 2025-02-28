package errors

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// stripANSI removes ANSI color codes from a string for easier testing
func stripANSI(s string) string {
	r := strings.NewReplacer(
		"\x1b[0m", "",
		"\x1b[1m", "",
		"\x1b[2m", "",
		"\x1b[31m", "",
		"\x1b[32m", "",
		"\x1b[33m", "",
		"\x1b[34m", "",
		"\x1b[35m", "",
		"\x1b[36m", "",
		"\x1b[37m", "",
		"\x1b[91m", "",
		"\x1b[92m", "",
		"\x1b[93m", "",
		"\x1b[94m", "",
		"\x1b[95m", "",
		"\x1b[96m", "",
		"\x1b[97m", "",
	)
	return r.Replace(s)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	t.Run("handles nil error", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		var exitCode int

		handler := NewHandler().
			WithWriter(&buf).
			WithExitFunc(func(code int) { exitCode = code })

		handler.Handle(nil)

		if buf.Len() > 0 {
			t.Errorf("Expected no output for nil error, got: %q", buf.String())
		}
		if exitCode != 0 {
			t.Errorf("Expected exit code 0 for nil error, got: %d", exitCode)
		}
	})

	t.Run("formats different error types", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name           string
			err            error
			expectedPrefix string
			expectedCode   int
		}{
			{
				name:           "validation error",
				err:            NewValidationError(nil, "Invalid input"),
				expectedPrefix: "Validation Error:",
				expectedCode:   ExitCodeValidationError,
			},
			{
				name:           "API error",
				err:            NewAPIError(nil, "API request failed"),
				expectedPrefix: "API Error:",
				expectedCode:   ExitCodeAPIError,
			},
			{
				name:           "not found error",
				err:            NewResourceNotFoundError(nil, "Resource not found"),
				expectedPrefix: "Not Found:",
				expectedCode:   ExitCodeNotFoundError,
			},
			{
				name:           "simple error",
				err:            fmt.Errorf("simple error"),
				expectedPrefix: "Error:",
				expectedCode:   ExitCodeGenericError,
			},
		}

		for _, tc := range testCases {
			tc := tc // Capture range variable
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var buf bytes.Buffer
				var exitCode int

				handler := NewHandler().
					WithWriter(&buf).
					WithExitFunc(func(code int) { exitCode = code })

				handler.Handle(tc.err)

				output := stripANSI(buf.String())
				if !strings.Contains(output, tc.expectedPrefix) {
					t.Errorf("Expected output to contain %q, got: %q", tc.expectedPrefix, output)
				}

				if exitCode != tc.expectedCode {
					t.Errorf("Expected exit code %d, got: %d", tc.expectedCode, exitCode)
				}
			})
		}
	})

	t.Run("includes suggestions in verbose mode", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		handler := NewHandler().
			WithWriter(&buf).
			WithExitFunc(func(int) {}).
			WithVerbose(true)

		suggestion1 := "Try using a different name"
		suggestion2 := "Check your spelling"
		err := NewValidationError(nil, "Invalid name", suggestion1, suggestion2)

		handler.Handle(err)

		output := stripANSI(buf.String())
		if !strings.Contains(output, suggestion1) {
			t.Errorf("Expected output to contain suggestion %q, got: %q", suggestion1, output)
		}
		if !strings.Contains(output, suggestion2) {
			t.Errorf("Expected output to contain suggestion %q, got: %q", suggestion2, output)
		}
	})

	t.Run("includes one suggestion in non-verbose mode", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		handler := NewHandler().
			WithWriter(&buf).
			WithExitFunc(func(int) {})

		suggestion1 := "Try using a different name"
		suggestion2 := "Check your spelling"
		err := NewValidationError(nil, "Invalid name", suggestion1, suggestion2)

		handler.Handle(err)

		output := stripANSI(buf.String())
		if !strings.Contains(output, suggestion1) {
			t.Errorf("Expected output to contain first suggestion %q, got: %q", suggestion1, output)
		}
		if strings.Contains(output, suggestion2) {
			t.Errorf("Expected output to NOT contain second suggestion in non-verbose mode, got: %q", output)
		}
	})

	t.Run("handles errors with details", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		handler := NewHandler().
			WithWriter(&buf).
			WithExitFunc(func(int) {})

		err := fmt.Errorf("something went wrong")
		operation := "fetching data"

		handler.HandleWithDetails(err, operation)

		output := stripANSI(buf.String())
		if !strings.Contains(output, "something went wrong") {
			t.Errorf("Expected output to contain error message, got: %q", output)
		}
		if !strings.Contains(output, operation) {
			t.Errorf("Expected output to contain operation details %q, got: %q", operation, output)
		}
	})

	t.Run("prints warnings", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		handler := NewHandler().
			WithWriter(&buf)

		warningMsg := "Something might be wrong"
		handler.PrintWarning(warningMsg)

		output := stripANSI(buf.String())
		if !strings.Contains(output, "Warning:") {
			t.Errorf("Expected output to contain 'Warning:', got: %q", output)
		}
		if !strings.Contains(output, warningMsg) {
			t.Errorf("Expected output to contain warning message %q, got: %q", warningMsg, output)
		}
	})

	t.Run("MessageForError returns formatted message", func(t *testing.T) {
		t.Parallel()

		err := NewValidationError(nil, "Invalid input")
		message := MessageForError(err)
		
		// Strip ANSI codes for testing
		plainMessage := stripANSI(message)
		
		if !strings.Contains(plainMessage, "Validation Error:") {
			t.Errorf("Expected message to contain error category, got: %q", plainMessage)
		}
		if !strings.Contains(plainMessage, "Invalid input") {
			t.Errorf("Expected message to contain error details, got: %q", plainMessage)
		}
	})

	t.Run("GetExitCodeForError returns correct code", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			err          error
			expectedCode int
		}{
			{
				name:         "nil error",
				err:          nil,
				expectedCode: ExitCodeSuccess,
			},
			{
				name:         "validation error",
				err:          NewValidationError(nil, ""),
				expectedCode: ExitCodeValidationError,
			},
			{
				name:         "API error",
				err:          NewAPIError(nil, ""),
				expectedCode: ExitCodeAPIError,
			},
			{
				name:         "not found error",
				err:          NewResourceNotFoundError(nil, ""),
				expectedCode: ExitCodeNotFoundError,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				code := GetExitCodeForError(tc.err)
				if code != tc.expectedCode {
					t.Errorf("Expected exit code %d, got: %d", tc.expectedCode, code)
				}
			})
		}
	})
}
