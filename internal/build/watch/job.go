package watch

import (
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// FormattedJob wraps a Buildkite job with watch-specific formatting and classification helpers.
type FormattedJob struct {
	buildkite.Job
}

// NewFormattedJob wraps a Buildkite job.
func NewFormattedJob(j buildkite.Job) FormattedJob {
	return FormattedJob{Job: j}
}

// DisplayName returns a human-readable name for a job.
func (j FormattedJob) DisplayName() string {
	if j.Name != "" {
		return j.Name
	}
	if j.Label != "" {
		return j.Label
	}
	return j.Type + " step"
}

// Duration returns the elapsed duration for a job.
func (j FormattedJob) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if j.FinishedAt != nil {
		end = j.FinishedAt.Time
	}
	return end.Sub(j.StartedAt.Time).Truncate(time.Second)
}

func (j FormattedJob) IsTerminalFailureState() bool {
	return j.State == "failed" || j.State == "timed_out" || j.State == "canceled" || j.State == "expired"
}

func (j FormattedJob) IsSoftFailed() bool {
	return j.SoftFailed
}

func (j FormattedJob) IsFailed() bool {
	return j.IsTerminalFailureState()
}
