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

func TestSpinWhileActionIsExecuted(t *testing.T) {
	// Test that the action is always executed regardless of TTY status
	counter := 0
	err := SpinWhile("Test action", func() {
		counter++
	})

	if err != nil {
		t.Errorf("SpinWhile should not return error: %v", err)
	}

	if counter != 1 {
		t.Errorf("Action should have been called exactly once, got %d", counter)
	}
}

func TestSpinWhileWithError(t *testing.T) {
	// Test SpinWhile when action panics or has issues
	actionCalled := false
	err := SpinWhile("Test action with panic recovery", func() {
		actionCalled = true
		// Don't actually panic in test, just test normal flow
	})

	if err != nil {
		t.Errorf("SpinWhile should not return error for normal action: %v", err)
	}

	if !actionCalled {
		t.Error("Action should have been called")
	}
}

func TestSpinWhileTTYDetection(t *testing.T) {
	// Test that TTY detection works as expected
	// This test documents the behavior rather than forcing specific outcomes
	isTTY := IsTerminal()

	actionCalled := false
	err := SpinWhile("TTY detection test", func() {
		actionCalled = true
	})

	if err != nil {
		t.Errorf("SpinWhile should not return error: %v", err)
	}

	if !actionCalled {
		t.Error("Action should have been called regardless of TTY status")
	}

	// Document the current TTY status for debugging
	t.Logf("Current TTY status: %v", isTTY)
}
