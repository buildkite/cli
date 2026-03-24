package watch

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestSummarize(t *testing.T) {
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
			name: "counts canceled",
			jobs: []buildkite.Job{
				{Type: "script", State: "canceled"},
			},
			want: JobSummary{Canceled: 1},
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
				{Type: "script", State: "expired"},
			},
			want: JobSummary{Waiting: 8},
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
			got := Summarize(buildkite.Build{Jobs: tt.jobs})
			if got != tt.want {
				t.Errorf("Summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
