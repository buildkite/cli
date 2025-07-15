package io

import (
	"testing"
)

func TestConfirmWithoutTTY(t *testing.T) {
	// Test that Confirm works without TTY and defaults to true
	confirmed := false
	err := Confirm(&confirmed, "Test confirmation")

	if err != nil {
		t.Errorf("Confirm should not return error: %v", err)
	}

	if !confirmed {
		t.Error("Confirm should default to true when no TTY is available")
	}
}

func TestConfirmWithFlag(t *testing.T) {
	// Test that Confirm respects pre-set flag
	confirmed := true
	err := Confirm(&confirmed, "Test confirmation")

	if err != nil {
		t.Errorf("Confirm should not return error: %v", err)
	}

	if !confirmed {
		t.Error("Confirm should respect pre-set flag")
	}
}
