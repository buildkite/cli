package preflight

import (
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestEvent_Operation(t *testing.T) {
	e := Event{
		Type:        EventOperation,
		Time:        time.Now(),
		PreflightID: "preflight-123",
		Title:       "Creating snapshot of working tree...",
	}

	if e.Type != EventOperation {
		t.Fatalf("expected EventOperation, got %q", e.Type)
	}
	if e.Title == "" {
		t.Fatal("expected Title to be set")
	}
	if e.BuildState != "" {
		t.Fatal("expected BuildState to be empty for operation event")
	}
}

func TestEvent_BuildStatus(t *testing.T) {
	e := Event{
		Type:        EventBuildStatus,
		Time:        time.Now(),
		PreflightID: "preflight-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
		BuildState:  "running",
		Jobs: &watch.JobSummary{
			Passed:  8,
			Running: 3,
		},
	}

	if e.Type != EventBuildStatus {
		t.Fatalf("expected EventBuildStatus, got %q", e.Type)
	}
	if e.BuildNumber != 42 {
		t.Fatalf("expected BuildNumber 42, got %d", e.BuildNumber)
	}
	if e.Jobs.Passed != 8 {
		t.Fatalf("expected 8 passed, got %d", e.Jobs.Passed)
	}
}

func TestEvent_JobFailure(t *testing.T) {
	e := Event{
		Type:        EventJobFailure,
		Time:        time.Now(),
		PreflightID: "preflight-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildState:  "failing",
		Job: &buildkite.Job{
			ID:    "job-1",
			Name:  "Lint",
			State: "failed",
		},
	}

	if e.Type != EventJobFailure {
		t.Fatalf("expected EventJobFailure, got %q", e.Type)
	}
	if e.Job == nil {
		t.Fatal("expected Job to be set")
	}
	if e.Job.ID != "job-1" {
		t.Fatalf("expected job ID job-1, got %q", e.Job.ID)
	}
}

func TestEvent_BuildSummaryStoppedEarly(t *testing.T) {
	buildCanceled := false
	e := Event{
		Type:          EventBuildSummary,
		Time:          time.Now(),
		PreflightID:   "preflight-123",
		Pipeline:      "buildkite/cli",
		BuildNumber:   42,
		BuildState:    "failing",
		Incomplete:    true,
		StopReason:    "build-failing",
		BuildCanceled: &buildCanceled,
	}

	if e.Type != EventBuildSummary {
		t.Fatalf("expected EventBuildSummary, got %q", e.Type)
	}
	if !e.Incomplete {
		t.Fatal("expected Incomplete to be set")
	}
	if e.StopReason != "build-failing" {
		t.Fatalf("expected stop reason build-failing, got %q", e.StopReason)
	}
	if e.BuildCanceled == nil || *e.BuildCanceled {
		t.Fatalf("expected BuildCanceled=false, got %#v", e.BuildCanceled)
	}
}
