package watch

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestTestTracker_Update(t *testing.T) {
	t.Run("reports new test changes", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-1",
					FailureReason: "expected 3, got 2",
				},
				Executions: []buildkite.BuildTestExecution{{
					ID:            "exec-1",
					Status:        "failed",
					FailureReason: "expected 3, got 2",
				}},
			},
		}

		newTestChanges := tracker.Update(tests)
		if len(newTestChanges) != 1 {
			t.Fatalf("expected 1 new test change, got %d", len(newTestChanges))
		}
		if newTestChanges[0].Name != "flaky test" {
			t.Errorf("expected 'flaky test', got %q", newTestChanges[0].Name)
		}
	})

	t.Run("does not re-report same execution", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				Executions: []buildkite.BuildTestExecution{{
					ID:            "exec-1",
					Status:        "failed",
					FailureReason: "expected 3, got 2",
				}},
			},
		}

		tracker.Update(tests)
		newTestChanges := tracker.Update(tests)
		if len(newTestChanges) != 0 {
			t.Errorf("expected 0 new test changes on second poll, got %d", len(newTestChanges))
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
				Executions: []buildkite.BuildTestExecution{{
					ID:            "exec-1",
					Status:        "failed",
					FailureReason: "first failure",
				}},
			},
		})

		newTestChanges := tracker.Update([]buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-2",
					FailureReason: "second failure",
				},
				Executions: []buildkite.BuildTestExecution{{
					ID:            "exec-2",
					Status:        "failed",
					FailureReason: "second failure",
				}},
			},
		})

		if len(newTestChanges) != 1 {
			t.Fatalf("expected 1 new test change, got %d", len(newTestChanges))
		}
		if newTestChanges[0].LatestFail == nil || newTestChanges[0].LatestFail.ID != "exec-2" {
			t.Errorf("expected latest fail exec-2, got %#v", newTestChanges[0].LatestFail)
		}
	})

	t.Run("skips tests without executions", func(t *testing.T) {
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
				Executions: []buildkite.BuildTestExecution{{
					ID:            "exec-1",
					Status:        "failed",
					FailureReason: "boom",
				}},
			},
		}

		newTestChanges := tracker.Update(tests)
		if len(newTestChanges) != 1 {
			t.Fatalf("expected 1 new test change, got %d", len(newTestChanges))
		}
		if newTestChanges[0].ID != "test-2" {
			t.Errorf("expected test-2, got %s", newTestChanges[0].ID)
		}
	})

	t.Run("handles multiple new test changes at once", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:         "test-1",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-1"},
				Executions: []buildkite.BuildTestExecution{{ID: "exec-1", Status: "failed"}},
			},
			{
				ID:         "test-2",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-2"},
				Executions: []buildkite.BuildTestExecution{{ID: "exec-2", Status: "failed"}},
			},
			{
				ID:         "test-3",
				LatestFail: &buildkite.BuildTestLatestFail{ID: "exec-3"},
				Executions: []buildkite.BuildTestExecution{{ID: "exec-3", Status: "failed"}},
			},
		}

		newTestChanges := tracker.Update(tests)
		if len(newTestChanges) != 3 {
			t.Fatalf("expected 3 new test changes, got %d", len(newTestChanges))
		}
	})

	t.Run("reports one test change when a test has multiple new executions", func(t *testing.T) {
		tracker := NewTestTracker()
		tests := []buildkite.BuildTest{
			{
				ID:   "test-1",
				Name: "flaky test",
				LatestFail: &buildkite.BuildTestLatestFail{
					ID:            "exec-2",
					FailureReason: "second failure",
				},
				Executions: []buildkite.BuildTestExecution{
					{ID: "exec-1", Status: "failed", FailureReason: "first failure"},
					{ID: "exec-2", Status: "failed", FailureReason: "second failure"},
				},
			},
		}

		newTestChanges := tracker.Update(tests)

		if len(newTestChanges) != 1 {
			t.Fatalf("expected 1 new test change, got %d", len(newTestChanges))
		}
		if newTestChanges[0].LatestFail == nil || newTestChanges[0].LatestFail.ID != "exec-2" {
			t.Fatalf("expected latest fail exec-2, got %#v", newTestChanges[0].LatestFail)
		}

		newTestChanges = tracker.Update(tests)
		if len(newTestChanges) != 0 {
			t.Fatalf("expected 0 new test changes on second poll, got %d", len(newTestChanges))
		}
	})
}
