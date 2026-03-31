package watch

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestTestTracker_Update(t *testing.T) {
	t.Run("reports new failures", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-1",
					FailureReason: "expected 3, got 2",
				},
			},
		}

		newFailures := tracker.Update(tests)
		if len(newFailures) != 1 {
			t.Fatalf("expected 1 new failure, got %d", len(newFailures))
		}
		if newFailures[0].Name != "flaky test" {
			t.Errorf("expected 'flaky test', got %q", newFailures[0].Name)
		}
	})

	t.Run("does not re-report same execution", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-1",
					FailureReason: "expected 3, got 2",
				},
			},
		}

		tracker.Update(tests)
		newFailures := tracker.Update(tests)
		if len(newFailures) != 0 {
			t.Errorf("expected 0 new failures on second poll, got %d", len(newFailures))
		}
	})

	t.Run("reports new execution for same test", func(t *testing.T) {
		tracker := NewTestTracker()
		tracker.Update([]buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-1",
					FailureReason: "first failure",
				},
			},
		})

		newFailures := tracker.Update([]buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-2",
					FailureReason: "second failure",
				},
			},
		})

		if len(newFailures) != 1 {
			t.Fatalf("expected 1 new failure, got %d", len(newFailures))
		}
		if newFailures[0].LatestFail.ID != "exec-2" {
			t.Errorf("expected exec-2, got %s", newFailures[0].LatestFail.ID)
		}
	})

	t.Run("skips tests without latest_fail", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{ID: "test-1", Name: "passing test"},
			{
				ID:   "test-2",
				Name: "failing test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-1",
					FailureReason: "boom",
				},
			},
		}

		newFailures := tracker.Update(tests)
		if len(newFailures) != 1 {
			t.Fatalf("expected 1 new failure, got %d", len(newFailures))
		}
		if newFailures[0].ID != "test-2" {
			t.Errorf("expected test-2, got %s", newFailures[0].ID)
		}
	})

	t.Run("handles multiple new failures at once", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:         "test-1",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-1"},
			},
			{
				ID:         "test-2",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-2"},
			},
			{
				ID:         "test-3",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-3"},
			},
		}

		newFailures := tracker.Update(tests)
		if len(newFailures) != 3 {
			t.Fatalf("expected 3 new failures, got %d", len(newFailures))
		}
	})
}
