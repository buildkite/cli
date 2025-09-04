package io

import (
	"testing"
)

func TestPromptForOneWithoutTTY(t *testing.T) {
	// Test that PromptForOne fails gracefully without TTY
	_, err := PromptForOne("pipeline", []string{"option1", "option2"})

	if err == nil {
		t.Error("PromptForOne should return error when no TTY is available")
	}

	expectedError := "cannot prompt for selection: no TTY available (use appropriate flags to specify the selection)"
	if err.Error() != expectedError {
		t.Errorf("Expected error message %q, got %q", expectedError, err.Error())
	}
}
