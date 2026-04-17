package preflight

import (
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
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

func TestTTYModelRender_IncludesTestFailureCount(t *testing.T) {
	m := newTTYModel()
	m.latest = Event{
		Title: "Watching build #42",
		Jobs: &watch.JobSummary{
			Passed:  8,
			Failed:  1,
			Running: 2,
		},
	}
	m.testFailures = 3

	got := stripANSI(m.render())
	for _, want := range []string{"Watching build #42", "8 passed", "1 failed job", "3 failed tests", "2 running"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestJobPresenterTTYBlock_SeparatesStatusAndCommand(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	block := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ttyBlock(scriptJob("job-1", "Lint", "failed", false, &startedAt, &finishedAt, &exitStatus))

	got := stripANSI(block)
	for _, want := range []string{"● job Lint", "│ failed with exit 1", "│ bk job log -b 42 -p buildkite/cli job-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestTestPresenterTTYBlock_UsesDiagnosticLayout(t *testing.T) {
	executionTime := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	block := testPresenter{}.ttyBlock(buildkite.BuildTest{
		Name:            "Test A",
		ExecutionsCount: 1,
		ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
			Failed: 1,
		},
		Executions: []buildkite.BuildTestExecution{{
			Status:        "failed",
			Location:      "./spec/example_spec.rb:10",
			FailureReason: "Failure/Error: expect(false).to eq(true)\nexpected: true\nactual: false",
			Timestamp:     &executionTime,
		}},
	})

	got := stripANSI(block)
	for _, want := range []string{
		"● test Test A",
		"│ ./spec/example_spec.rb:10",
		"│ 1 attempt (0 passed, 1 failed)",
		"│ Failure/Error: expect(false).to eq(true)",
		"│ expected: true",
		"│ actual: false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
}
