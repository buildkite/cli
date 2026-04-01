package preflight

import (
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// EventType identifies the kind of preflight event.
type EventType string

const (
	EventStatus     EventType = "status"
	EventJobFailure EventType = "job_failure"
)

// Event is the single data model emitted by a preflight run.
// Renderers project events differently by output mode (TTY, text, JSON).
type Event struct {
	Type EventType
	Time time.Time

	PreflightID string

	// Operation is set for pre-build status events (e.g. "Creating snapshot…").
	Operation string

	Pipeline    string
	BuildNumber int
	BuildURL    string
	BuildState  string

	Jobs watch.JobSummary

	// Job is set for job_failure events.
	Job *buildkite.Job
}
