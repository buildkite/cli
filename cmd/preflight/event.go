package preflight

import (
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// EventType identifies the kind of preflight event.
type EventType string

const (
	EventOperation   EventType = "operation"
	EventBuildStatus EventType = "build_status"
	EventJobFailure  EventType = "job_failure"
	EventTestFailure EventType = "test_failure"
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

	// TestFailures is set for test_failure events.
	TestFailures []buildkite.BuildTest `json:"test_failures,omitempty"`
}
