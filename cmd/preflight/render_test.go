package preflight

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

var ansiCodesPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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
	indent := strings.Repeat(" ", len("10:30:00 "))
	expected := "10:30:00 Creating snapshot of working tree...:\n" +
		indent + "Commit: abc1234567\n"
	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
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
	indent := strings.Repeat(" ", len("10:30:00 "))
	expected := "10:30:00 Created snapshot of working tree...:\n" +
		indent + "Commit: abc1234567\n" +
		indent + "Ref:    refs/heads/bk/preflight/abc123\n" +
		indent + "Files:  2 changed\n" +
		indent + "  ~ app/controllers/jobs_controller.rb\n" +
		indent + "  ~ db/structure.sql\n"
	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestFormatTimestampedDetail_UsesLeftAlignedTimestampIndent(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	got := formatTimestampedDetail("Created snapshot of working tree...", "Commit: abc1234567\nRef: refs/heads/bk/preflight/abc123", now)

	indent := strings.Repeat(" ", len("10:30:00 "))
	expected := "10:30:00 Created snapshot of working tree...:\n" +
		indent + "Commit: abc1234567\n" +
		indent + "Ref: refs/heads/bk/preflight/abc123"
	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestFormatTimestampedBlock_IndentsContinuationLines(t *testing.T) {
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	got := formatTimestampedBlock("  ❌ test: react/jsx-no-bind\n    Location: .eslintrc.js:120\n    Got 193 failures and 0 errors.", now)

	indent := strings.Repeat(" ", len("10:30:00 "))
	expected := "10:30:00   ❌ test: react/jsx-no-bind\n" +
		indent + "    Location: .eslintrc.js:120\n" +
		indent + "    Got 193 failures and 0 errors."
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

func TestTestPresenter_Line_FailedAttemptIncludesHistoryAndFailureDetails(t *testing.T) {
	t.Parallel()

	line := testPresenter{}.Line(buildkite.BuildTest{
		Name:     "Pipelines::ShardMigration::DeleteOrganizationFromShardWorker with more than BATCH_SIZE records for a shard that needs cleaning",
		Location: "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181",
		Executions: []buildkite.BuildTestExecution{
			{
				Status:        "failed",
				Location:      "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181",
				FailureReason: "Failure/Error: expect(empty_tables).to eq({})",
			},
			{
				Status:        "failed",
				Location:      "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181",
				FailureReason: "Failure/Error: expect(empty_tables).to eq({})",
			},
		},
	})

	got := stripANSI(line)

	if !strings.Contains(got, "✗ ✗ Pipelines::ShardMigration::DeleteOrganizationFromShardWorker") {
		t.Fatalf("expected cumulative failure history, got %q", got)
	}
	if !strings.Contains(got, "Location: ./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181") {
		t.Fatalf("expected location detail, got %q", got)
	}
	if !strings.Contains(got, "Failure/Error: expect(empty_tables).to eq({})") {
		t.Fatalf("expected failure reason, got %q", got)
	}
	if strings.Contains(got, "BATCH_SIZE records for a shard that needs cleaning") {
		t.Fatalf("expected long name to be truncated, got %q", got)
	}
}

func TestTestPresenter_Line_PassedAttemptOnlyShowsHistoryLine(t *testing.T) {
	t.Parallel()

	line := testPresenter{}.Line(buildkite.BuildTest{
		Name:     "Test A",
		Location: "./spec/example_spec.rb:10",
		Executions: []buildkite.BuildTestExecution{
			{Status: "failed", FailureReason: "Failure/Error: expect(false).to eq(true)", Location: "./spec/example_spec.rb:10"},
			{Status: "failed", FailureReason: "Failure/Error: expect(false).to eq(true)", Location: "./spec/example_spec.rb:10"},
			{Status: "passed", Location: "./spec/example_spec.rb:10"},
		},
	})

	got := stripANSI(line)

	if strings.Contains(got, "Location:") {
		t.Fatalf("expected passed attempt to omit location detail, got %q", got)
	}
	if strings.Contains(got, "Failure/Error:") {
		t.Fatalf("expected passed attempt to omit failure detail, got %q", got)
	}
	if got != "✗ ✗ ✓ Test A" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func stripANSI(s string) string {
	return ansiCodesPattern.ReplaceAllString(s, "")
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

func TestPlainRenderer_Render_BuildSummaryPassed(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	if err := r.Render(Event{
		Type:        EventBuildSummary,
		Time:        time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildState:  "passed",
		PassedJobs: []buildkite.Job{
			{ID: "job-1", Name: "Lint", Type: "script", State: "passed"},
			{ID: "job-2", Name: "Test", Type: "script", State: "passed"},
		},
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "✅ Preflight Passed") {
		t.Fatalf("expected passed header, got %q", got)
	}
	if !strings.Contains(got, "Lint") {
		t.Fatalf("expected passed job name, got %q", got)
	}
	if !strings.Contains(got, "Test") {
		t.Fatalf("expected passed job name, got %q", got)
	}
}

func TestPlainRenderer_Render_BuildSummaryFailed(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	exitOne := 1
	if err := r.Render(Event{
		Type:        EventBuildSummary,
		Time:        time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildState:  "failed",
		FailedJobs: []buildkite.Job{
			{ID: "job-1", Name: "Lint", Type: "script", State: "failed", ExitStatus: &exitOne},
		},
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "❌ Preflight Failed") {
		t.Fatalf("expected failed header, got %q", got)
	}
	if !strings.Contains(got, "Lint") {
		t.Fatalf("expected hard-failed job name, got %q", got)
	}
	if strings.Contains(got, "Optional check") {
		t.Fatalf("soft-failed job should not appear in summary, got %q", got)
	}
}

func TestJSONRenderer_Render_BuildSummaryPassed(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	if err := r.Render(Event{
		Type:        EventBuildSummary,
		Time:        time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
		PreflightID: "pfid-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildState:  "passed",
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got["type"] != "build_summary" {
		t.Fatalf("expected type build_summary, got %v", got["type"])
	}
	if got["build_state"] != "passed" {
		t.Fatalf("expected build_state=passed, got %v", got["build_state"])
	}
	if got["failed_jobs"] != nil {
		t.Fatalf("expected no failed_jobs for passing build, got %v", got["failed_jobs"])
	}
}

func TestJSONRenderer_Render_BuildSummaryFailed(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

	exitOne := 1
	if err := r.Render(Event{
		Type:        EventBuildSummary,
		Time:        time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
		PreflightID: "pfid-123",
		Pipeline:    "buildkite/cli",
		BuildNumber: 42,
		BuildState:  "failed",
		FailedJobs: []buildkite.Job{
			{ID: "job-1", Name: "Lint", Type: "script", State: "failed", ExitStatus: &exitOne},
		},
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got["build_state"] != "failed" {
		t.Fatalf("expected build_state=failed, got %v", got["build_state"])
	}
	failedJobs, ok := got["failed_jobs"].([]any)
	if !ok || len(failedJobs) != 1 {
		t.Fatalf("expected 1 failed job, got %v", got["failed_jobs"])
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{1 * time.Second, "1 second"},
		{30 * time.Second, "30 seconds"},
		{1 * time.Minute, "1 minute"},
		{1*time.Minute + 1*time.Second, "1 minute 1 second"},
		{1*time.Minute + 30*time.Second, "1 minute 30 seconds"},
		{90 * time.Second, "1 minute 30 seconds"},
		{10 * time.Minute, "10 minutes"},
		{10*time.Minute + 23*time.Second, "10 minutes 23 seconds"},
		{1 * time.Hour, "1 hour"},
		{1*time.Hour + 1*time.Minute, "1 hour 1 minute"},
		{2 * time.Hour, "2 hours"},
		{2*time.Hour + 5*time.Minute, "2 hours 5 minutes"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
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
