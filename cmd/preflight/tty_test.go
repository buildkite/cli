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
		width    int
		event    Event
		contains []string
	}{
		{
			name:  "passed build no jobs",
			width: 0,
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "passed",
				BuildNumber: 42,
				BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
			},
			contains: []string{"─────", "Build #42", "https://buildkite.com/buildkite/cli/builds/42"},
		},
		{
			name:  "passed build with jobs",
			width: 0,
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "passed",
				BuildNumber: 42,
				BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
				PassedJobs: []buildkite.Job{
					{ID: "j1", Name: "Lint", Type: "script", State: "passed"},
					{ID: "j2", Name: "Test", Type: "script", State: "passed"},
				},
			},
			contains: []string{"Build #42", "https://buildkite.com/buildkite/cli/builds/42", "✔ Lint", "✔ Test"},
		},
		{
			name:  "failed build no jobs",
			width: 0,
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "failed",
				BuildNumber: 42,
				BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
			},
			contains: []string{"─────", "Build #42", "https://buildkite.com/buildkite/cli/builds/42"},
		},
		{
			name:  "failed build with jobs",
			width: 0,
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "failed",
				Pipeline:    "buildkite/cli",
				BuildNumber: 42,
				BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
				FailedJobs: func() []buildkite.Job {
					exit := 1
					return []buildkite.Job{
						{ID: "j1", Name: "Lint", Type: "script", State: "failed", ExitStatus: &exit},
					}
				}(),
			},
			contains: []string{"Build #42", "https://buildkite.com/buildkite/cli/builds/42", "✗", "Lint", "failed with exit 1"},
		},
		{
			name:  "wraps build url on narrow terminals",
			width: 24,
			event: Event{
				Type:        EventBuildSummary,
				BuildState:  "passed",
				BuildNumber: 42,
				BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
			},
			contains: []string{"Build #42", "https://buildkite.com/", "buildkite/cli/builds/42"},
		},
		{
			name: "build with tests",
			event: Event{
				Type:       EventBuildSummary,
				BuildState: "failed",
				Tests: map[string]internalpreflight.SummaryTestSuite{
					"rspec": {Passed: 47, Failed: 2, Skipped: 3},
				},
				Failures: []internalpreflight.SummaryTestFailure{{
					SuiteSlug: "rspec",
					Name:      "AuthService.validateToken handles expired tokens",
					Location:  "src/auth.test.ts:89",
					Message:   "Expected 'expired' but got 'invalid'",
				}},
			},
			contains: []string{"Tests X", "rspec: 47 passed, 2 failed, 3 skipped", "FAIL [rspec] — src/auth.test.ts:89 — AuthService.validateToken handles expired tokens — Expected 'expired' but got 'invalid'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (ttyModel{width: tt.width}).buildSummaryView(tt.event)
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
