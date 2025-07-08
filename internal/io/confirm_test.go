package io

import (
	"os"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name        string
		confirmed   bool
		isTTY       bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "already confirmed via flag",
			confirmed:   true,
			isTTY:       false,
			expectError: false,
		},
		{
			name:        "not confirmed and not TTY",
			confirmed:   false,
			isTTY:       false,
			expectError: true,
			errorMsg:    "confirmation required but not running in a terminal; use -y or --yes to confirm",
		},
		{
			name:        "not confirmed and TTY (interactive test skipped)",
			confirmed:   false,
			isTTY:       true,
			expectError: false, // We'll skip this test as it requires user interaction
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "not confirmed and TTY (interactive test skipped)" {
				t.Skip("Skipping interactive test")
			}

			// Mock TTY detection by temporarily redirecting stdout
			if !tt.isTTY {
				// Create a pipe to simulate non-TTY
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				defer w.Close()

				// Temporarily replace stdout
				oldStdout := os.Stdout
				os.Stdout = w
				defer func() {
					os.Stdout = oldStdout
				}()
			}

			confirmed := tt.confirmed
			err := Confirm(&confirmed, "Test confirmation")

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q but got %q", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
