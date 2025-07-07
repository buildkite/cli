package io

import (
	"testing"
)

func TestSpinWhileWithoutTTY(t *testing.T) {
	// Test that SpinWhile works without TTY
	actionCalled := false
	err := SpinWhile("Test action", func() {
		actionCalled = true
	})

	if err != nil {
		t.Errorf("SpinWhile should not return error: %v", err)
	}

	if !actionCalled {
		t.Error("Action should have been called")
	}
}
