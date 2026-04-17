package preflight

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
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

func TestPlainRenderer_Render_TestFailure(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

	now := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	executionTime := buildkite.Timestamp{Time: now}
	r.Render(Event{
		Type: EventTestFailure,
		Time: now,
		TestFailures: []buildkite.BuildTest{{
			Name:            "Test A",
			ExecutionsCount: 1,
			ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
				Failed: 1,
			},
			Executions: []buildkite.BuildTestExecution{{
				Status:        "failed",
				Location:      "./spec/example_spec.rb:10",
				FailureReason: "Failure/Error: expect(false).to eq(true)",
				Timestamp:     &executionTime,
			}},
		}},
	})

	got := out.String()
	if ansiCodesPattern.MatchString(got) {
		t.Fatalf("expected plain output without ANSI codes, got %q", got)
	}

	indent := strings.Repeat(" ", len("10:31:00 "))
	expected := "10:31:00 ✗ Test A\n" +
		indent + "    1 attempt (0 passed, 1 failed) — ./spec/example_spec.rb:10\n" +
		indent + "    Failure/Error: expect(false).to eq(true)\n"
	if got != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, got)
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

func TestTestPresenter_Line_FailedAttemptIncludesSummaryAndFailureDetails(t *testing.T) {
	t.Parallel()
	older := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)}
	newer := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	line := testPresenter{}.Line(buildkite.BuildTest{
		Name:            "Pipelines::ShardMigration::DeleteOrganizationFromShardWorker with more than BATCH_SIZE records for a shard that needs cleaning",
		Location:        "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181",
		ExecutionsCount: 2,
		ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
			Failed: 2,
		},
		Executions: []buildkite.BuildTestExecution{
			{
				Status:        "failed",
				Location:      "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181",
				FailureReason: "Failure/Error: expect(empty_tables).to eq({})",
				Timestamp:     &newer,
			},
			{
				Status:        "failed",
				Location:      "./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:182",
				FailureReason: "Failure/Error: expect(empty_tables).to eq({})",
				Timestamp:     &older,
			},
		},
	})

	if ansiCodesPattern.MatchString(line) {
		t.Fatalf("expected plain line without ANSI codes, got %q", line)
	}

	got := line

	if !strings.Contains(got, "✗ Pipelines::ShardMigration::DeleteOrgan...records for a shard that needs cleaning") {
		t.Fatalf("expected long name to preserve the start and end, got %q", got)
	}
	if !strings.Contains(got, "2 attempts (0 passed, 2 failed) — ./spec/workers/pipelines/shard_migration/delete_organization_from_shard_worker_spec.rb:181") {
		t.Fatalf("expected location detail, got %q", got)
	}
	if !strings.Contains(got, "Failure/Error: expect(empty_tables).to eq({})") {
		t.Fatalf("expected failure reason, got %q", got)
	}
	if strings.Contains(got, "BATCH_SIZE records for a shard that needs cleaning") {
		t.Fatalf("expected long name to be truncated, got %q", got)
	}
}

func TestFormatTestStatusIcon_UsesLatestExecution(t *testing.T) {
	t.Parallel()
	newest := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	execution := &buildkite.BuildTestExecution{Status: "passed", Timestamp: &newest}
	icon := formatTestStatusIcon(execution, false)

	if got, want := icon, "✓"; got != want {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

func TestFormatTestStatusIcon_NilExecution(t *testing.T) {
	t.Parallel()

	icon := formatTestStatusIcon(nil, false)

	if got, want := icon, "?"; got != want {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

func TestTestAttemptCounts_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	counts := testAttemptCounts(buildkite.BuildTest{
		ExecutionsCount: 5,
		ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
			Passed: 3,
			Failed: 2,
		},
		Executions: []buildkite.BuildTestExecution{{Status: "failed"}},
	})

	if got, want := counts, "5 attempts (3 passed, 2 failed)"; got != want {
		t.Fatalf("counts = %q, want %q", got, want)
	}
}

func TestTestPresenter_Line_PassedLatestAttemptOnlyShowsSummaryLine(t *testing.T) {
	t.Parallel()
	oldest := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 29, 0, 0, time.UTC)}
	middle := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)}
	newest := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	line := testPresenter{}.Line(buildkite.BuildTest{
		Name:            "Test A",
		Location:        "./spec/example_spec.rb:10",
		ExecutionsCount: 3,
		ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
			Passed: 1,
			Failed: 2,
		},
		Executions: []buildkite.BuildTestExecution{
			{Status: "passed", Location: "./spec/example_spec.rb:10", Timestamp: &newest},
			{Status: "failed", FailureReason: "Failure/Error: expect(false).to eq(true)", Location: "./spec/example_spec.rb:10", Timestamp: &oldest},
			{Status: "failed", FailureReason: "Failure/Error: expect(false).to eq(true)", Location: "./spec/example_spec.rb:10", Timestamp: &middle},
		},
	})

	if ansiCodesPattern.MatchString(line) {
		t.Fatalf("expected plain line without ANSI codes, got %q", line)
	}

	got := line

	if strings.Contains(got, "./spec/example_spec.rb:10") {
		t.Fatalf("expected passed attempt to omit location detail, got %q", got)
	}
	if strings.Contains(got, "Failure/Error:") {
		t.Fatalf("expected passed attempt to omit failure detail, got %q", got)
	}
}

func TestLatestTestExecution_PicksNewestTimestamp(t *testing.T) {
	t.Parallel()
	older := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)}
	newer := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	execution := latestTestExecution(buildkite.BuildTest{
		Executions: []buildkite.BuildTestExecution{
			{Status: "failed", Location: "./spec/example_spec.rb:11", Timestamp: &older},
			{Status: "passed", Location: "./spec/example_spec.rb:12", Timestamp: &newer},
			{Status: "failed", Location: "./spec/example_spec.rb:10"},
		},
	})

	if execution == nil {
		t.Fatal("expected execution to be present")
	}
	if got, want := execution.Status, "passed"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := execution.Location, "./spec/example_spec.rb:12"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func TestLatestTestExecution_IgnoresExecutionsWithoutTimestamps(t *testing.T) {
	t.Parallel()

	execution := latestTestExecution(buildkite.BuildTest{
		Executions: []buildkite.BuildTestExecution{
			{Status: "failed", Location: "./spec/example_spec.rb:10"},
			{Status: "passed", Location: "./spec/example_spec.rb:11"},
		},
	})

	if execution != nil {
		t.Fatalf("expected nil execution, got %#v", execution)
	}
}

func TestTestPresenter_ColoredLine_AddsANSIStyles(t *testing.T) {
	t.Parallel()
	executionTime := buildkite.Timestamp{Time: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)}

	line := testPresenter{}.ColoredLine(buildkite.BuildTest{
		Name:            "Test A",
		ExecutionsCount: 1,
		ExecutionsCountByResult: buildkite.BuildTestExecutionsCount{
			Failed: 1,
		},
		Executions: []buildkite.BuildTestExecution{{
			Status:        "failed",
			Location:      "./spec/example_spec.rb:10",
			FailureReason: "Failure/Error: expect(false).to eq(true)",
			Timestamp:     &executionTime,
		}},
	})

	if !ansiCodesPattern.MatchString(line) {
		t.Fatalf("expected colored line with ANSI codes, got %q", line)
	}

	got := stripANSI(line)
	if !strings.Contains(got, "✗ Test A") {
		t.Fatalf("expected colored line to preserve headline text content, got %q", got)
	}
	if !strings.Contains(got, "1 attempt (0 passed, 1 failed) — ./spec/example_spec.rb:10") {
		t.Fatalf("expected colored line to preserve text content, got %q", got)
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
		BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
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
	if !strings.Contains(got, "Build #42: https://buildkite.com/buildkite/cli/builds/42") {
		t.Fatalf("expected build URL in summary, got %q", got)
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
		BuildURL:    "https://buildkite.com/buildkite/cli/builds/42",
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
	if !strings.Contains(got, "Build #42: https://buildkite.com/buildkite/cli/builds/42") {
		t.Fatalf("expected build URL in summary, got %q", got)
	}
	if !strings.Contains(got, "Build Failures:") {
		t.Fatalf("expected build failures section, got %q", got)
	}
	if !strings.Contains(got, "Lint") {
		t.Fatalf("expected hard-failed job name, got %q", got)
	}
	if strings.Contains(got, "Optional check") {
		t.Fatalf("soft-failed job should not appear in summary, got %q", got)
	}
}

func TestPlainRenderer_Render_BuildSummaryIncludesTests(t *testing.T) {
	var out bytes.Buffer
	r := newPlainRenderer(&out)

		if err := r.Render(Event{
			Type:       EventBuildSummary,
			Time:       time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
			BuildState: "failed",
			Tests: map[string]internalpreflight.SummaryTestRun{
				"run-go":    {RunID: "run-go", SuiteSlug: "go", Passed: 12, Failed: 1, Skipped: 0},
				"run-rspec": {RunID: "run-rspec", SuiteSlug: "rspec", Passed: 47, Failed: 2, Skipped: 3},
			},
			Failures: []internalpreflight.SummaryTestFailure{{
				RunID:     "run-rspec",
				SuiteSlug: "rspec",
				Name:      "AuthService.validateToken handles expired tokens",
				Location:  "src/auth.test.ts:89",
				Message:   "Expected 'expired' but got 'invalid'",
		}},
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := out.String()
		if !strings.Contains(got, "Tests X") {
			t.Fatalf("expected tests section, got %q", got)
		}
		if !strings.Contains(got, "go: 12 passed, 1 failed, 0 skipped") {
			t.Fatalf("expected go test summary, got %q", got)
		}
		if !strings.Contains(got, "rspec: 47 passed, 2 failed, 3 skipped") {
			t.Fatalf("expected rspec test summary, got %q", got)
		}
		if !strings.Contains(got, "FAIL [rspec] — src/auth.test.ts:89 — AuthService.validateToken handles expired tokens — Expected 'expired' but got 'invalid'") {
			t.Fatalf("expected failure header from endpoint summary, got %q", got)
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

func TestJSONRenderer_Render_BuildSummaryIncludesTests(t *testing.T) {
	var out bytes.Buffer
	r := newJSONRenderer(&out)

		if err := r.Render(Event{
			Type:        EventBuildSummary,
			Time:        time.Date(2025, 1, 15, 10, 32, 0, 0, time.UTC),
			PreflightID: "pfid-123",
			BuildState:  "failed",
			Tests: map[string]internalpreflight.SummaryTestRun{
				"run-rspec": {RunID: "run-rspec", SuiteSlug: "rspec", Passed: 47, Failed: 2, Skipped: 3},
			},
			Failures: []internalpreflight.SummaryTestFailure{{
				RunID:     "run-rspec",
				SuiteSlug: "rspec",
				Name:      "AuthService.validateToken handles expired tokens",
				Location:  "src/auth.test.ts:89",
				Message:   "Expected 'expired' but got 'invalid'",
		}},
	}); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	tests, ok := got["tests"].(map[string]any)
	if !ok {
		t.Fatalf("expected tests object, got %v", got["tests"])
	}
		rspec, ok := tests["run-rspec"].(map[string]any)
		if !ok {
			t.Fatalf("expected rspec summary, got %v", tests["run-rspec"])
		}
		if rspec["passed"] != float64(47) || rspec["failed"] != float64(2) || rspec["skipped"] != float64(3) {
			t.Fatalf("unexpected rspec summary: %v", rspec)
		}
		if rspec["suite_slug"] != "rspec" || rspec["run_id"] != "run-rspec" {
			t.Fatalf("unexpected rspec identifiers: %v", rspec)
		}
		failures, ok := got["failures"].([]any)
		if !ok || len(failures) != 1 {
			t.Fatalf("expected one failure, got %v", got["failures"])
		}
		failure, ok := failures[0].(map[string]any)
		if !ok {
			t.Fatalf("expected failure object, got %v", failures[0])
		}
		if failure["suite_slug"] != "rspec" || failure["run_id"] != "run-rspec" {
			t.Fatalf("unexpected failure identifiers: %v", failure)
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
