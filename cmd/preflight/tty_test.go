package preflight

import (
	"strings"
	"testing"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestBuildSummaryView_ReturnsOutput(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		contains []string
	}{
		{
			name: "passed build no jobs",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "passed",
			},
			contains: []string{"─────"},
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
			contains: []string{"✔ Lint", "✔ Test"},
		},
		{
			name: "failed build no jobs",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "failed",
			},
			contains: []string{"─────"},
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
			contains: []string{"✗", "Lint", "failed with exit 1"},
		},
		{
			name: "build with tests",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "failed",
				Tests: map[string]internalpreflight.ShowTestSuite{
					"rspec": {Passed: 47, Failed: 2, Skipped: 3},
				},
			},
			contains: []string{"rspec tests: 47 passed, 2 failed, 3 skipped"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSummaryView(tt.event)
			if got == "" {
				t.Fatal("expected non-empty summary view")
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in output:\n%s", want, got)
				}
			}
		})
	}
}
