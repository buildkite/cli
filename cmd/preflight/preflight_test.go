package preflight

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
)

func TestRecordPollingError(t *testing.T) {
	t.Run("increments count and retries before max", func(t *testing.T) {
		count := 0

		err := recordPollingError(errors.New("temporary failure"), &count, "polling for preflight build")
		if err != nil {
			t.Fatalf("recordPollingError returned unexpected error: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected count to be 1, got %d", count)
		}
	})

	t.Run("returns internal error on max consecutive failures", func(t *testing.T) {
		count := maxPollingErrors - 1

		err := recordPollingError(errors.New("temporary failure"), &count, "fetching build status")
		if err == nil {
			t.Fatal("expected an error on max consecutive failures")
		}
		if count != maxPollingErrors {
			t.Fatalf("expected count to be %d, got %d", maxPollingErrors, count)
		}
		if !errors.Is(err, bkErrors.ErrInternal) {
			t.Fatalf("expected internal error category, got: %v", err)
		}
		expected := fmt.Sprintf("fetching build status failed %d times", maxPollingErrors)
		if got := err.Error(); got == "" || !strings.Contains(got, expected) {
			t.Fatalf("expected error to contain %q, got %q", expected, got)
		}
	})

	t.Run("resets count on successful poll", func(t *testing.T) {
		count := 7

		err := recordPollingError(nil, &count, "")
		if err != nil {
			t.Fatalf("recordPollingError returned unexpected error: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected count to reset to 0, got %d", count)
		}
	})

	t.Run("resets after success then retries from zero", func(t *testing.T) {
		count := maxPollingErrors - 1

		if err := recordPollingError(nil, &count, ""); err != nil {
			t.Fatalf("unexpected reset error: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected count to reset to 0, got %d", count)
		}

		err := recordPollingError(errors.New("temporary failure"), &count, "polling for preflight build")
		if err != nil {
			t.Fatalf("expected retryable error after reset, got: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected count to be 1 after new failure, got %d", count)
		}
	})
}
