package testutil

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TeaTestOptions contains configuration options for testing tea models
type TeaTestOptions struct {
	Timeout time.Duration
}

// DefaultTeaTestOptions provides default options for testing tea models
func DefaultTeaTestOptions() TeaTestOptions {
	return TeaTestOptions{
		Timeout: time.Second,
	}
}

// TestTeaModel creates a test model and runs common assertions
func TestTeaModel(t *testing.T, model tea.Model, expected string, opts TeaTestOptions) {
	t.Helper()

	testModel := teatest.NewTestModel(t, model)

	// Wait for expected output
	teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(expected))
	})

	// Wait for model to finish with timeout
	testModel.WaitFinished(t, teatest.WithFinalTimeout(opts.Timeout))
}

// AssertTeaOutput creates a test model and asserts that the final output contains the expected string
func AssertTeaOutput(t *testing.T, model tea.Model, expected string) {
	t.Helper()

	testModel := teatest.NewTestModel(t, model)
	out, err := io.ReadAll(testModel.FinalOutput(t))
	if err != nil {
		t.Errorf("Failed to get stdout: %v", err)
	}

	if !bytes.Contains(out, []byte(expected)) {
		t.Errorf("Expected output to contain %q, got %q", expected, out)
	}
}

// Contains checks if a byte slice contains a string
func Contains(bts []byte, s string) bool {
	return bytes.Contains(bts, []byte(s))
}
