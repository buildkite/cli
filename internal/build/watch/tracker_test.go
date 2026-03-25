package watch

import (
	"fmt"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestJobTracker_Update(t *testing.T) {
	t.Run("first poll reports failures", func(t *testing.T) {
		tracker := NewJobTracker()
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "passed"},
				{ID: "2", Type: "script", State: "failed"},
				{ID: "3", Type: "script", State: "running"},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].ID != "2" {
			t.Errorf("expected job 2, got %s", status.NewlyFailed[0].ID)
		}
	})

	t.Run("same data second poll has no newly failed", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "failed"},
			},
		})

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "failed"},
			},
		})

		if len(status.NewlyFailed) != 0 {
			t.Errorf("expected 0 newly failed, got %d", len(status.NewlyFailed))
		}
	})

	t.Run("running to failed transition", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "running"},
			},
		})

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "failed"},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].State != "failed" {
			t.Errorf("expected state failed, got %s", status.NewlyFailed[0].State)
		}
	})

	t.Run("soft failed reported", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "running"},
			},
		})

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "passed", SoftFailed: true},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if !status.NewlyFailed[0].SoftFailed {
			t.Error("expected SoftFailed to be true")
		}
	})

	t.Run("timed out reported as failed", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "running"},
			},
		})

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "timed_out"},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].State != "timed_out" {
			t.Errorf("expected state timed_out, got %s", status.NewlyFailed[0].State)
		}
	})

	t.Run("skips non-script and broken jobs", func(t *testing.T) {
		tracker := NewJobTracker()
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "waiter", State: "failed"},
				{ID: "2", Type: "manual", State: "failed"},
				{ID: "3", Type: "script", State: "broken"},
				{ID: "4", Type: "script", State: "failed"},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].ID != "4" {
			t.Errorf("expected job 4, got %s", status.NewlyFailed[0].ID)
		}
	})

	t.Run("running jobs capped at MaxRunning", func(t *testing.T) {
		tracker := NewJobTracker()
		var jobs []buildkite.Job
		for i := 0; i < 15; i++ {
			jobs = append(jobs, buildkite.Job{
				ID:    fmt.Sprintf("job-%d", i),
				Type:  "script",
				State: "running",
			})
		}

		status := tracker.Update(buildkite.Build{Jobs: jobs})

		if status.TotalRunning != 15 {
			t.Errorf("expected TotalRunning 15, got %d", status.TotalRunning)
		}
		if len(status.Running) != MaxRunning {
			t.Errorf("expected Running capped at %d, got %d", MaxRunning, len(status.Running))
		}
	})

	t.Run("new job appears mid-build", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "running"},
			},
		})

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "running"},
				{ID: "2", Type: "script", State: "failed"},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].ID != "2" {
			t.Errorf("expected job 2, got %s", status.NewlyFailed[0].ID)
		}
	})

	t.Run("summary counts are correct", func(t *testing.T) {
		tracker := NewJobTracker()
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "passed"},
				{ID: "2", Type: "script", State: "failed"},
				{ID: "3", Type: "script", State: "running"},
				{ID: "4", Type: "script", State: "scheduled"},
				{ID: "5", Type: "script", State: "running"},
			},
		})

		if status.Summary.Passed != 1 {
			t.Errorf("expected 1 passed, got %d", status.Summary.Passed)
		}
		if status.Summary.Failed != 1 {
			t.Errorf("expected 1 failed, got %d", status.Summary.Failed)
		}
		if status.Summary.Running != 2 {
			t.Errorf("expected 2 running, got %d", status.Summary.Running)
		}
		if status.Summary.Scheduled != 1 {
			t.Errorf("expected 1 scheduled, got %d", status.Summary.Scheduled)
		}
	})

	t.Run("failed job includes exit status and duration", func(t *testing.T) {
		tracker := NewJobTracker()
		now := time.Now()
		start := now.Add(-5 * time.Second)
		exitCode := 2

		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{
					ID:         "1",
					Type:       "script",
					Name:       "lint",
					State:      "failed",
					ExitStatus: &exitCode,
					StartedAt:  &buildkite.Timestamp{Time: start},
					FinishedAt: &buildkite.Timestamp{Time: now},
				},
			},
		})

		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed, got %d", len(status.NewlyFailed))
		}
		fj := status.NewlyFailed[0]
		if fj.Name != "lint" {
			t.Errorf("expected name lint, got %s", fj.Name)
		}
		if fj.ExitStatus == nil || *fj.ExitStatus != 2 {
			t.Errorf("expected exit status 2, got %v", fj.ExitStatus)
		}
		if fj.Duration != 5*time.Second {
			t.Errorf("expected duration 5s, got %s", fj.Duration)
		}
	})
}

func TestJobDisplayName(t *testing.T) {
	tests := []struct {
		name string
		job  buildkite.Job
		want string
	}{
		{"uses Name", buildkite.Job{Name: "lint"}, "lint"},
		{"uses Label when no Name", buildkite.Job{Label: "test"}, "test"},
		{"falls back to type", buildkite.Job{Type: "script"}, "script step"},
		{"Name takes precedence", buildkite.Job{Name: "lint", Label: "test"}, "lint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JobDisplayName(tt.job)
			if got != tt.want {
				t.Errorf("JobDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJobDuration(t *testing.T) {
	t.Run("no start time", func(t *testing.T) {
		d := JobDuration(buildkite.Job{})
		if d != 0 {
			t.Errorf("expected 0, got %s", d)
		}
	})

	t.Run("with start and finish", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		finish := time.Date(2025, 1, 1, 0, 0, 12, 0, time.UTC)
		d := JobDuration(buildkite.Job{
			StartedAt:  &buildkite.Timestamp{Time: start},
			FinishedAt: &buildkite.Timestamp{Time: finish},
		})
		if d != 12*time.Second {
			t.Errorf("expected 12s, got %s", d)
		}
	})

	t.Run("truncates to seconds", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		finish := time.Date(2025, 1, 1, 0, 0, 3, 500_000_000, time.UTC)
		d := JobDuration(buildkite.Job{
			StartedAt:  &buildkite.Timestamp{Time: start},
			FinishedAt: &buildkite.Timestamp{Time: finish},
		})
		if d != 3*time.Second {
			t.Errorf("expected 3s, got %s", d)
		}
	})
}
