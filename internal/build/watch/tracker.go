package watch

import (
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

const MaxRunning = 10

// trackedJob holds a job and its lifecycle state across polls.
type trackedJob struct {
	Job       buildkite.Job
	PrevState string // state from previous poll, "" if first seen
	Reported  bool   // true once surfaced to caller as failed
}

// FailedJob is a snapshot of a job that has failed, for rendering.
type FailedJob struct {
	Name       string
	ID         string
	State      string
	ExitStatus *int
	Duration   time.Duration
	SoftFailed bool
}

// BuildStatus is the output of JobTracker.Update().
type BuildStatus struct {
	NewlyFailed  []FailedJob
	Running      []buildkite.Job
	TotalRunning int
	Summary      JobSummary
	Build        buildkite.Build
}

// JobTracker tracks job state changes across polls.
type JobTracker struct {
	jobs map[string]*trackedJob
}

// NewJobTracker creates a new JobTracker.
func NewJobTracker() *JobTracker {
	return &JobTracker{
		jobs: make(map[string]*trackedJob),
	}
}

// Update processes a build and returns the current status with any state changes.
func (t *JobTracker) Update(b buildkite.Build) BuildStatus {
	var status BuildStatus
	status.Build = b

	var running []buildkite.Job

	for _, j := range b.Jobs {
		if j.Type != "script" || j.State == "broken" {
			continue
		}

		tj, exists := t.jobs[j.ID]
		if !exists {
			tj = &trackedJob{}
			t.jobs[j.ID] = tj
		} else {
			tj.PrevState = tj.Job.State
		}
		tj.Job = j

		if isFailedJob(j) && !wasFailedJob(tj.PrevState, false) && !tj.Reported {
			status.NewlyFailed = append(status.NewlyFailed, newFailedJob(j))
			tj.Reported = true
		}

		if isActiveState(j.State) {
			running = append(running, j)
		}
	}

	status.Summary = Summarize(b)
	status.TotalRunning = len(running)
	if len(running) > MaxRunning {
		status.Running = running[:MaxRunning]
	} else {
		status.Running = running
	}

	return status
}

func newFailedJob(j buildkite.Job) FailedJob {
	return FailedJob{
		Name:       JobDisplayName(j),
		ID:         j.ID,
		State:      j.State,
		ExitStatus: j.ExitStatus,
		Duration:   JobDuration(j),
		SoftFailed: j.SoftFailed,
	}
}

// JobDisplayName returns a human-readable name for a job.
func JobDisplayName(j buildkite.Job) string {
	if j.Name != "" {
		return j.Name
	}
	if j.Label != "" {
		return j.Label
	}
	return j.Type + " step"
}

// JobDuration returns the elapsed duration for a job.
func JobDuration(j buildkite.Job) time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if j.FinishedAt != nil {
		end = j.FinishedAt.Time
	}
	return end.Sub(j.StartedAt.Time).Truncate(time.Second)
}

// isFailedJob mirrors the build page's hasJobFailed / isJobTerminated logic:
// a script job is failed if it finished without passing (failed, timed_out,
// canceled, expired) or was soft-failed.
func isFailedJob(j buildkite.Job) bool {
	return j.State == "failed" || j.State == "timed_out" || j.State == "canceled" || j.State == "expired" || j.SoftFailed
}

func wasFailedJob(state string, softFailed bool) bool {
	return state == "failed" || state == "timed_out" || state == "canceled" || state == "expired" || softFailed
}

func isActiveState(state string) bool {
	return state == "running" || state == "canceling" || state == "timing_out"
}
