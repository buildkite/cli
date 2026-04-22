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
				{ID: "1", Type: "script", State: "failed", SoftFailed: true},
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

	t.Run("returns all running jobs", func(t *testing.T) {
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
		if len(status.Running) != 15 {
			t.Errorf("expected Running to include all 15 jobs, got %d", len(status.Running))
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
		if duration := NewFormattedJob(fj).Duration(); duration != 5*time.Second {
			t.Errorf("expected duration 5s, got %s", duration)
		}
	})
}

func TestJobTracker_Update_RetriedJobs(t *testing.T) {
	t.Run("superseded job still reported as newly failed", func(t *testing.T) {
		tracker := NewJobTracker()
		// Poll 1: job already failed and retried (e.g. automatic retry)
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "running", RetriesCount: 1},
			},
		})
		if len(status.NewlyFailed) != 1 {
			t.Fatalf("expected 1 newly failed (even though superseded), got %d", len(status.NewlyFailed))
		}
		if status.NewlyFailed[0].ID != "orig" {
			t.Errorf("expected orig, got %s", status.NewlyFailed[0].ID)
		}

		// Poll 2: same state, not re-reported
		status = tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "running", RetriesCount: 1},
			},
		})
		if len(status.NewlyFailed) != 0 {
			t.Errorf("expected 0 newly failed on re-poll, got %d", len(status.NewlyFailed))
		}
	})

	t.Run("retry passed detected when retry job reaches passed", func(t *testing.T) {
		tracker := NewJobTracker()
		// Poll 1: job fails
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed"},
			},
		})

		// Poll 2: original retried, retry running
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "running", RetriesCount: 1},
			},
		})

		// Poll 3: retry passes
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
			},
		})
		if len(status.NewlyRetryPassed) != 1 {
			t.Fatalf("expected 1 retry passed, got %d", len(status.NewlyRetryPassed))
		}
		if status.NewlyRetryPassed[0].ID != "retry-1" {
			t.Errorf("expected retry-1, got %s", status.NewlyRetryPassed[0].ID)
		}
	})

	t.Run("retry passed reported only once", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed"},
			},
		})
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
			},
		})
		// Second poll with same passed state
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
			},
		})
		if len(status.NewlyRetryPassed) != 0 {
			t.Errorf("expected 0 retry passed on second poll, got %d", len(status.NewlyRetryPassed))
		}
	})

	t.Run("chained retries: second retry passes", func(t *testing.T) {
		tracker := NewJobTracker()
		// Poll 1: original fails
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed"},
			},
		})
		// Poll 2: original retried, first retry also fails
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "failed", RetriesCount: 1},
			},
		})
		// Poll 3: first retry retried, second retry passes
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-2", RetriesCount: 1},
				{ID: "retry-2", Type: "script", State: "passed", RetriesCount: 2},
			},
		})
		if len(status.NewlyRetryPassed) != 1 {
			t.Fatalf("expected 1 retry passed, got %d", len(status.NewlyRetryPassed))
		}
		if status.NewlyRetryPassed[0].ID != "retry-2" {
			t.Errorf("expected retry-2, got %s", status.NewlyRetryPassed[0].ID)
		}
	})

	t.Run("summary excludes superseded jobs", func(t *testing.T) {
		tracker := NewJobTracker()
		status := tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
			},
		})
		if status.Summary.Failed != 0 {
			t.Errorf("expected 0 failed (superseded excluded), got %d", status.Summary.Failed)
		}
		if status.Summary.Passed != 1 {
			t.Errorf("expected 1 passed, got %d", status.Summary.Passed)
		}
	})
}

func TestJobTracker_FailedJobs(t *testing.T) {
	t.Run("returns hard failed jobs and excludes soft failures", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "failed"},
				{ID: "2", Type: "script", State: "running"},
			},
		})

		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "script", State: "failed"},
				{ID: "2", Type: "script", State: "failed", SoftFailed: true},
			},
		})

		failedJobs := tracker.FailedJobs()
		if len(failedJobs) != 1 {
			t.Fatalf("expected 1 failed job, got %d", len(failedJobs))
		}
		if failedJobs[0].ID != "1" {
			t.Errorf("expected failed job 1, got %s", failedJobs[0].ID)
		}
	})

	t.Run("excludes non-script, broken, and soft-failed jobs", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "1", Type: "waiter", State: "failed"},
				{ID: "2", Type: "script", State: "broken"},
				{ID: "3", Type: "script", State: "failed"},
				{ID: "4", Type: "script", State: "failed", SoftFailed: true},
			},
		})

		failedJobs := tracker.FailedJobs()
		if len(failedJobs) != 1 {
			t.Fatalf("expected 1 failed job, got %d", len(failedJobs))
		}
		if failedJobs[0].ID != "3" {
			t.Errorf("expected failed job 3, got %s", failedJobs[0].ID)
		}
	})

	t.Run("excludes superseded (retried) jobs", func(t *testing.T) {
		tracker := NewJobTracker()
		tracker.Update(buildkite.Build{
			Jobs: []buildkite.Job{
				{ID: "orig", Type: "script", State: "failed", Retried: true, RetriedInJobID: "retry-1"},
				{ID: "still-failed", Type: "script", State: "failed"},
				{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
			},
		})

		failedJobs := tracker.FailedJobs()
		if len(failedJobs) != 1 {
			t.Fatalf("expected 1 failed job (superseded excluded), got %d", len(failedJobs))
		}
		if failedJobs[0].ID != "still-failed" {
			t.Errorf("expected still-failed, got %s", failedJobs[0].ID)
		}
	})
}

func TestJobTracker_PassedJobs_ExcludesSuperseded(t *testing.T) {
	tracker := NewJobTracker()
	tracker.Update(buildkite.Build{
		Jobs: []buildkite.Job{
			{ID: "orig", Type: "script", State: "passed", Retried: true, RetriedInJobID: "retry-1"},
			{ID: "retry-1", Type: "script", State: "passed", RetriesCount: 1},
		},
	})

	jobs := tracker.PassedJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 passed job (superseded excluded), got %d", len(jobs))
	}
	if jobs[0].ID != "retry-1" {
		t.Errorf("expected retry-1, got %s", jobs[0].ID)
	}
}

func TestJobTracker_PassedJobs_SortedByStartTime(t *testing.T) {
	tracker := NewJobTracker()
	t1 := buildkite.Timestamp{Time: time.Date(2025, 1, 1, 0, 0, 10, 0, time.UTC)}
	t2 := buildkite.Timestamp{Time: time.Date(2025, 1, 1, 0, 0, 5, 0, time.UTC)}
	tracker.Update(buildkite.Build{
		Jobs: []buildkite.Job{
			{ID: "late", Type: "script", State: "passed", StartedAt: &t1},
			{ID: "early", Type: "script", State: "passed", StartedAt: &t2},
			{ID: "no-start", Type: "script", State: "passed"},
		},
	})

	jobs := tracker.PassedJobs()
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	wantOrder := []string{"early", "late", "no-start"}
	for i, id := range wantOrder {
		if jobs[i].ID != id {
			t.Errorf("position %d: got %s, want %s", i, jobs[i].ID, id)
		}
	}
}

func TestJobTracker_FailedJobs_SortedByStartTime(t *testing.T) {
	tracker := NewJobTracker()
	t1 := buildkite.Timestamp{Time: time.Date(2025, 1, 1, 0, 0, 20, 0, time.UTC)}
	t2 := buildkite.Timestamp{Time: time.Date(2025, 1, 1, 0, 0, 10, 0, time.UTC)}
	tracker.Update(buildkite.Build{
		Jobs: []buildkite.Job{
			{ID: "b", Type: "script", State: "failed", StartedAt: &t1},
			{ID: "a", Type: "script", State: "failed", StartedAt: &t2},
		},
	})

	jobs := tracker.FailedJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].ID != "a" || jobs[1].ID != "b" {
		t.Errorf("expected [a, b], got [%s, %s]", jobs[0].ID, jobs[1].ID)
	}
}

func TestJobTracker_Summarize(t *testing.T) {
	tracker := NewJobTracker()

	tests := []struct {
		name string
		jobs []buildkite.Job
		want JobSummary
	}{
		{
			name: "empty build",
			jobs: nil,
			want: JobSummary{},
		},
		{
			name: "skips non-script jobs",
			jobs: []buildkite.Job{
				{Type: "waiter", State: "passed"},
				{Type: "manual", State: "blocked"},
			},
			want: JobSummary{},
		},
		{
			name: "counts passed",
			jobs: []buildkite.Job{
				{Type: "script", State: "passed"},
				{Type: "script", State: "passed"},
			},
			want: JobSummary{Passed: 2},
		},
		{
			name: "counts soft failed separately",
			jobs: []buildkite.Job{
				{Type: "script", State: "failed", SoftFailed: true},
				{Type: "script", State: "passed"},
			},
			want: JobSummary{Passed: 1, SoftFailed: 1},
		},
		{
			name: "counts failed and timed_out",
			jobs: []buildkite.Job{
				{Type: "script", State: "failed"},
				{Type: "script", State: "timed_out"},
			},
			want: JobSummary{Failed: 2},
		},
		{
			name: "counts running states",
			jobs: []buildkite.Job{
				{Type: "script", State: "running"},
				{Type: "script", State: "canceling"},
				{Type: "script", State: "timing_out"},
			},
			want: JobSummary{Running: 3},
		},
		{
			name: "counts canceled as failed",
			jobs: []buildkite.Job{
				{Type: "script", State: "canceled"},
			},
			want: JobSummary{Failed: 1},
		},
		{
			name: "counts expired as failed",
			jobs: []buildkite.Job{
				{Type: "script", State: "expired"},
			},
			want: JobSummary{Failed: 1},
		},
		{
			name: "counts skipped and broken",
			jobs: []buildkite.Job{
				{Type: "script", State: "skipped"},
				{Type: "script", State: "broken"},
			},
			want: JobSummary{Skipped: 2},
		},
		{
			name: "counts blocked states",
			jobs: []buildkite.Job{
				{Type: "script", State: "blocked"},
				{Type: "script", State: "blocked_failed"},
			},
			want: JobSummary{Blocked: 2},
		},
		{
			name: "counts scheduled states",
			jobs: []buildkite.Job{
				{Type: "script", State: "scheduled"},
				{Type: "script", State: "assigned"},
				{Type: "script", State: "accepted"},
				{Type: "script", State: "reserved"},
			},
			want: JobSummary{Scheduled: 4},
		},
		{
			name: "counts waiting states",
			jobs: []buildkite.Job{
				{Type: "script", State: "waiting"},
				{Type: "script", State: "waiting_failed"},
				{Type: "script", State: "pending"},
				{Type: "script", State: "limited"},
				{Type: "script", State: "limiting"},
				{Type: "script", State: "platform_limited"},
				{Type: "script", State: "platform_limiting"},
			},
			want: JobSummary{Waiting: 7},
		},
		{
			name: "ignores unknown states",
			jobs: []buildkite.Job{
				{Type: "script", State: "passed"},
				{Type: "script", State: "something_new"},
			},
			want: JobSummary{Passed: 1},
		},
		{
			name: "mixed build",
			jobs: []buildkite.Job{
				{Type: "script", State: "passed"},
				{Type: "script", State: "failed"},
				{Type: "script", State: "running"},
				{Type: "script", State: "scheduled"},
				{Type: "waiter", State: "passed"},
			},
			want: JobSummary{Passed: 1, Failed: 1, Running: 1, Scheduled: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tracker.summarize(buildkite.Build{Jobs: tt.jobs})
			if got != tt.want {
				t.Errorf("summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestJobSummary_String(t *testing.T) {
	tests := []struct {
		name    string
		summary JobSummary
		want    string
	}{
		{
			name:    "empty summary",
			summary: JobSummary{},
			want:    "",
		},
		{
			name:    "single field",
			summary: JobSummary{Passed: 3},
			want:    "3 passed",
		},
		{
			name:    "multiple fields in order",
			summary: JobSummary{Passed: 2, Failed: 1, Running: 3},
			want:    "2 passed, 1 failed, 3 running",
		},
		{
			name:    "soft failed shown separately",
			summary: JobSummary{Passed: 3, Failed: 1, SoftFailed: 2},
			want:    "3 passed, 1 failed, 2 soft failed",
		},
		{
			name:    "all fields",
			summary: JobSummary{Passed: 1, Failed: 2, SoftFailed: 3, Running: 4, Scheduled: 5, Blocked: 6, Skipped: 7, Waiting: 8},
			want:    "1 passed, 2 failed, 3 soft failed, 4 running, 5 scheduled, 6 blocked, 7 skipped, 8 waiting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJob_DisplayName(t *testing.T) {
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
			got := NewFormattedJob(tt.job).DisplayName()
			if got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJob_Duration(t *testing.T) {
	t.Run("no start time", func(t *testing.T) {
		d := NewFormattedJob(buildkite.Job{}).Duration()
		if d != 0 {
			t.Errorf("expected 0, got %s", d)
		}
	})

	t.Run("with start and finish", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		finish := time.Date(2025, 1, 1, 0, 0, 12, 0, time.UTC)
		d := NewFormattedJob(buildkite.Job{
			StartedAt:  &buildkite.Timestamp{Time: start},
			FinishedAt: &buildkite.Timestamp{Time: finish},
		}).Duration()
		if d != 12*time.Second {
			t.Errorf("expected 12s, got %s", d)
		}
	})

	t.Run("truncates to seconds", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		finish := time.Date(2025, 1, 1, 0, 0, 3, 500_000_000, time.UTC)
		d := NewFormattedJob(buildkite.Job{
			StartedAt:  &buildkite.Timestamp{Time: start},
			FinishedAt: &buildkite.Timestamp{Time: finish},
		}).Duration()
		if d != 3*time.Second {
			t.Errorf("expected 3s, got %s", d)
		}
	})
}
