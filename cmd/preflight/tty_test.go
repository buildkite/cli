package preflight

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestBuildSummaryCmd_ReturnsCmd(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{
			name: "passed build no jobs",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "passed",
			},
		},
		{
			name: "passed build with jobs",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "passed",
				PassedJobs: []buildkite.Job{
					{ID: "j1", Name: "Lint", Type: "script", State: "passed"},
					{ID: "j2", Name: "Test", Type: "script", State: "passed"},
				},
			},
		},
		{
			name: "failed build no jobs",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "failed",
			},
		},
		{
			name: "failed build with jobs",
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "failed",
				Pipeline:    "buildkite/cli",
				BuildNumber: 42,
				FailedJobs: func() []buildkite.Job {
					exit := 1
					return []buildkite.Job{
						{ID: "j1", Name: "Lint", Type: "script", State: "failed", ExitStatus: &exit},
					}
				}(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cmd := buildSummaryCmd(tt.event); cmd == nil {
				t.Fatal("expected non-nil tea.Cmd")
			}
		})
	}
}
