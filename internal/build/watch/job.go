package watch

import (
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// JobHelper wraps a Buildkite job with watch-specific helper methods.
type JobHelper struct {
	buildkite.Job
}

// NewJobHelper wraps a Buildkite job for helper-style accessors.
func NewJobHelper(j buildkite.Job) JobHelper {
	return JobHelper{Job: j}
}

// DisplayName returns a human-readable name for a job.
func (j JobHelper) DisplayName() string {
	if j.Name != "" {
		return j.Name
	}
	if j.Label != "" {
		return j.Label
	}
	return j.Type + " step"
}

// Duration returns the elapsed duration for a job.
func (j JobHelper) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if j.FinishedAt != nil {
		end = j.FinishedAt.Time
	}
	return end.Sub(j.StartedAt.Time).Truncate(time.Second)
}

func (j JobHelper) IsTerminalFailureState() bool {
	return j.State == "failed" || j.State == "timed_out" || j.State == "canceled" || j.State == "expired"
}

func (j JobHelper) IsSoftFailed() bool {
	return j.SoftFailed
}

func (j JobHelper) IsFailed() bool {
	return j.IsTerminalFailureState()
}
