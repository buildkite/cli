package preflight

import (
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// EventType identifies the kind of preflight event.
type EventType string

const (
	EventOperation    EventType = "operation"
	EventBuildStatus  EventType = "build_status"
	EventJobFailure   EventType = "job_failure"
	EventBuildSummary EventType = "build_summary"
	EventTestFailure  EventType = "test_failure"
)

// Event is the single data model emitted by a preflight run.
// Renderers project events differently by output mode (TTY, text, JSON).
type Event struct {
	Type EventType `json:"type"`
	Time time.Time `json:"timestamp"`

	PreflightID string `json:"preflight_id,omitempty"`

	// Title is the primary status text shown in the TTY dynamic area.
	Title string `json:"title,omitempty"`

	// Detail is supplementary information printed to the scrollback log.
	Detail string `json:"detail,omitempty"`

	Pipeline    string `json:"pipeline,omitempty"`
	BuildNumber int    `json:"build_number,omitempty"`
	BuildURL    string `json:"build_url,omitempty"`
	BuildState  string `json:"build_state,omitempty"`

	Jobs *watch.JobSummary `json:"jobs,omitempty"`

	// Job is set for job_failure events.
	Job *buildkite.Job `json:"job,omitempty"`

	// FailedJobs is set for build_summary events when the build failed. Contains hard-failed jobs only (soft failures excluded).
	FailedJobs []buildkite.Job `json:"failed_jobs,omitempty"`

	// PassedJobs is set for build_summary events when the build passed and has 10 or fewer jobs.
	PassedJobs []buildkite.Job `json:"passed_jobs,omitempty"`

	// Duration is set for build_summary events. Total elapsed time of the preflight run.
	Duration time.Duration `json:"duration_ns,omitempty"`

	// TestFailures is set for test_failure events.
	TestFailures []buildkite.BuildTest `json:"test_failures,omitempty"`

	// Tests is set for build_summary events when test analytics data is available.
	Tests map[string]*TestSuiteSummary `json:"tests,omitempty"`
}

// TestSuiteSummary aggregates test results for a single suite (run/label grouping).
type TestSuiteSummary struct {
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Skipped  int           `json:"skipped"`
	Failures []TestFailure `json:"failures,omitempty"`
}

// TestFailure describes a single test whose every execution failed.
type TestFailure struct {
	Name          string                      `json:"name"`
	Location      string                      `json:"location,omitempty"`
	Message       string                      `json:"message,omitempty"`
	FailureReason string                      `json:"failure_reason,omitempty"`
	FailureDetail []buildkite.FailureExpanded `json:"failure_detail,omitempty"`
}
