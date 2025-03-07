package testutil

import (
	"errors"
	"strings"
	"testing"
)

// AssertErrorContains checks if an error contains expected text
func AssertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()

	if err == nil {
		t.Errorf("Expected an error containing %q, but got nil", expected)
		return
	}

	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error to contain %q, got %q", expected, err.Error())
	}
}

// AssertErrorIs checks if an error matches the expected error
func AssertErrorIs(t *testing.T, err error, expected error) {
	t.Helper()

	if err == nil {
		t.Errorf("Expected an error of type %v, but got nil", expected)
		return
	}

	if !errors.Is(err, expected) {
		t.Errorf("Expected error of type %v, got %v", expected, err)
	}
}

// AssertEqual checks if two values are equal
func AssertEqual[T comparable](t *testing.T, actual, expected T, message string) {
	t.Helper()

	if actual != expected {
		t.Errorf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertNoError asserts that no error was returned
func AssertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}
