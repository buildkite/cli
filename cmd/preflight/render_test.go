package preflight

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestPlainRenderer_Render_Operation(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r.Render(Event{
		Type:  EventOperation,
		Time:  now,
		Title: "Creating snapshot of working tree...",
	})

	got := out.String()
	if !strings.Contains(got, "10:30:00") {
		t.Fatalf("expected timestamp, got %q", got)
	}
	if !strings.Contains(got, "Creating snapshot of working tree...") {
		t.Fatalf("expected title text, got %q", got)
	}
}

func TestPlainRenderer_Render_OperationWithDetail(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r.Render(Event{
		Type:   EventOperation,
		Time:   now,
		Title:  "Creating snapshot of working tree...",
		Detail: "Commit: abc1234567",
	})

	got := out.String()
	if !strings.Contains(got, "Creating snapshot of working tree...") {
		t.Fatalf("expected title text, got %q", got)
	}
	if !strings.Contains(got, "Commit: abc1234567") {
		t.Fatalf("expected detail text, got %q", got)
	}
}

func TestPlainRenderer_Render_OperationWithMultiLineDetail(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r.Render(Event{
		Type:  EventOperation,
		Time:  now,
		Title: "Created snapshot of working tree...",
		Detail: "Commit: abc1234567\nRef:    refs/heads/bk/preflight/abc123\nFiles:  2 changed\n" +
			"  ~ app/controllers/jobs_controller.rb\n  ~ db/structure.sql",
	})

	got := out.String()
	indent := strings.Repeat(" ", len("[10:30:00] "))
	expected := "[10:30:00] Created snapshot of working tree...:\n" +
		"Commit: abc1234567\n" +
		indent + "Ref:    refs/heads/bk/preflight/abc123\n" +
		indent + "Files:  2 changed\n" +
		indent + "  ~ app/controllers/jobs_controller.rb\n" +
		indent + "  ~ db/structure.sql\n"
	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestPlainRenderer_Render_BuildStatus(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 5, 0, time.UTC)
	r.Render(Event{
		Type:        EventBuildStatus,
		Time:        now,
		BuildNumber: 42,
		BuildState:  "running",
		Jobs:        &watch.JobSummary{Passed: 8, Running: 3},
	})

	got := out.String()
	if !strings.Contains(got, "Build #42 running") {
		t.Fatalf("expected build status line, got %q", got)
	}
	if !strings.Contains(got, "8 passed") {
		t.Fatalf("expected job summary, got %q", got)
	}
}

func TestPlainRenderer_Render_BuildStatusDeduplicates(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 5, 0, time.UTC)
	e := Event{
		Type:        EventBuildStatus,
		Time:        now,
		BuildNumber: 42,
		BuildState:  "running",
		Jobs:        &watch.JobSummary{Running: 3},
	}

	r.Render(e)
	r.Render(e)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (deduplicated), got %d: %v", len(lines), lines)
	}
}

func TestPlainRenderer_Render_JobFailure(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	exitOne := 1
	r.Render(Event{
		Type: EventJobFailure,
		Time: now,
		Job: &buildkite.Job{
			ID:         "job-1",
			Name:       "Lint",
			Type:       "script",
			State:      "failed",
			ExitStatus: &exitOne,
		},
	})

	got := out.String()
	if !strings.Contains(got, "10:31:00") {
		t.Fatalf("expected timestamp, got %q", got)
	}
	if !strings.Contains(got, "Lint") {
		t.Fatalf("expected job name, got %q", got)
	}
	if !strings.Contains(got, "job-1") {
		t.Fatalf("expected job ID, got %q", got)
	}
}

func TestJSONRenderer_Render_Operation(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r.Render(Event{
		Type:        EventOperation,
		Time:        now,
		PreflightID: "pfid-123",
		Title:       "Creating snapshot of working tree...",
	})

	var got Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Type != EventOperation {
		t.Fatalf("expected type %q, got %q", EventOperation, got.Type)
	}
	if got.Title != "Creating snapshot of working tree..." {
		t.Fatalf("expected title text, got %q", got.Title)
	}
	if got.PreflightID != "pfid-123" {
		t.Fatalf("expected preflight ID, got %q", got.PreflightID)
	}
}

func TestJSONRenderer_Render_BuildStatus(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 5, 0, time.UTC)
	r.Render(Event{
		Type:        EventBuildStatus,
		Time:        now,
		PreflightID: "pfid-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
		BuildState:  "running",
		Jobs:        &watch.JobSummary{Passed: 8, Running: 3},
	})

	var got Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.BuildNumber != 42 {
		t.Fatalf("expected build number 42, got %d", got.BuildNumber)
	}
	if got.BuildState != "running" {
		t.Fatalf("expected build state running, got %q", got.BuildState)
	}
	if got.Jobs.Passed != 8 {
		t.Fatalf("expected 8 passed, got %d", got.Jobs.Passed)
	}
}

func TestJSONRenderer_Render_JobFailure(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	exitOne := 1
	r.Render(Event{
		Type:        EventJobFailure,
		Time:        now,
		PreflightID: "pfid-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		Job: &buildkite.Job{
			ID:         "job-1",
			Name:       "Lint",
			State:      "failed",
			ExitStatus: &exitOne,
		},
	})

	var got Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Type != EventJobFailure {
		t.Fatalf("expected type %q, got %q", EventJobFailure, got.Type)
	}
	if got.Job == nil {
		t.Fatal("expected job to be present")
	}
	if got.Job.ID != "job-1" {
		t.Fatalf("expected job ID job-1, got %q", got.Job.ID)
	}
	if got.Job.Name != "Lint" {
		t.Fatalf("expected job name Lint, got %q", got.Job.Name)
	}
	if got.Job.ExitStatus == nil || *got.Job.ExitStatus != 1 {
		t.Fatalf("expected exit status 1, got %v", got.Job.ExitStatus)
	}
}

func TestJSONRenderer_Render_MultipleEvents_JSONL(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	r.Render(Event{Type: EventOperation, Time: now, Title: "step 1"})
	r.Render(Event{Type: EventOperation, Time: now, Title: "step 2"})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestNewRenderer_NonFileWriterDefaultsToPlain(t *testing.T) {
	var out bytes.Buffer
	r := newRenderer(&out, false, false, func() {})
	if _, ok := r.(*plainRenderer); !ok {
		t.Fatalf("expected *plainRenderer when stdout is a non-file io.Writer, got %T", r)
	}
}

func TestNewRenderer_TextModeForcesPlain(t *testing.T) {
	var out bytes.Buffer
	r := newRenderer(&out, false, true, func() {})
	if _, ok := r.(*plainRenderer); !ok {
		t.Fatalf("expected *plainRenderer when --text is set, got %T", r)
	}
}

func TestNewRenderer_JSONModeReturnsJSON(t *testing.T) {
	var out bytes.Buffer
	r := newRenderer(&out, true, false, func() {})
	if _, ok := r.(*jsonRenderer); !ok {
		t.Fatalf("expected *jsonRenderer when --json is set, got %T", r)
	}
}

func scriptJob(id, name, state string, softFailed bool, startedAt, finishedAt *buildkite.Timestamp, exitStatus *int) buildkite.Job {
	return buildkite.Job{
		ID:         id,
		Name:       name,
		Type:       "script",
		State:      state,
		SoftFailed: softFailed,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		ExitStatus: exitStatus,
	}
}
