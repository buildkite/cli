package watch

import (
	"fmt"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// trackedJob holds a job and its lifecycle state across polls.
type trackedJob struct {
	Job       buildkite.Job
	PrevState string // state from previous poll, "" if first seen
	Reported  bool   // true once surfaced to caller as failed
}

// JobSummary aggregates job counts by high-level state.
type JobSummary struct {
	Passed     int
	Failed     int
	SoftFailed int
	Running    int
	Scheduled  int
	Blocked    int
	Skipped    int
	Waiting    int
}

// String returns a human-readable summary of non-zero job counts.
func (s JobSummary) String() string {
	type entry struct {
		count int
		label string
	}
	entries := []entry{
		{s.Passed, "passed"},
		{s.Failed, "failed"},
		{s.SoftFailed, "soft failed"},
		{s.Running, "running"},
		{s.Scheduled, "scheduled"},
		{s.Blocked, "blocked"},
		{s.Skipped, "skipped"},
		{s.Waiting, "waiting"},
	}
	var parts []string
	for _, e := range entries {
		if e.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", e.count, e.label))
		}
	}
	return strings.Join(parts, ", ")
}

// BuildStatus is the output of JobTracker.Update().
type BuildStatus struct {
	NewlyFailed  []buildkite.Job
	Running      []buildkite.Job
	TotalRunning int
	Summary      JobSummary
	Build        buildkite.Build
}

// FailedJobs separates terminal failures by whether they are soft-failed.
type FailedJobs struct {
	Hard []buildkite.Job
	Soft []buildkite.Job
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
		job := NewFormattedJob(j)

		tj, exists := t.jobs[j.ID]
		if !exists {
			tj = &trackedJob{}
			t.jobs[j.ID] = tj
		} else {
			tj.PrevState = tj.Job.State
		}
		tj.Job = j

		prevJob := NewFormattedJob(buildkite.Job{State: tj.PrevState})
		if job.IsFailed() && !prevJob.IsTerminalFailureState() && !tj.Reported {
			status.NewlyFailed = append(status.NewlyFailed, j)
			tj.Reported = true
		}

		if isActiveState(j.State) {
			running = append(running, j)
		}
	}

	status.Summary = t.summarize(b)
	status.TotalRunning = len(running)
	status.Running = running

	return status
}

// FailedJobs returns all failed jobs separated into hard and soft failures.
func (t *JobTracker) FailedJobs() FailedJobs {
	var result FailedJobs
	for _, tj := range t.jobs {
		job := NewFormattedJob(tj.Job)
		if job.IsFailed() {
			if job.IsSoftFailed() {
				result.Soft = append(result.Soft, tj.Job)
				continue
			}
			result.Hard = append(result.Hard, tj.Job)
		}
	}
	return result
}

func (t *JobTracker) summarize(b buildkite.Build) JobSummary {
	var s JobSummary
	for _, j := range b.Jobs {
		if j.Type != "script" {
			continue
		}
		job := NewFormattedJob(j)
		if job.IsSoftFailed() {
			s.SoftFailed++
			continue
		}
		switch j.State {
		case "running", "canceling", "timing_out":
			s.Running++
		case "passed":
			s.Passed++
		case "failed", "timed_out", "canceled", "expired":
			s.Failed++
		case "skipped", "broken":
			s.Skipped++
		case "blocked", "blocked_failed":
			s.Blocked++
		case "scheduled", "assigned", "accepted", "reserved":
			s.Scheduled++
		case "waiting", "waiting_failed",
			"pending", "limited", "limiting",
			"platform_limited", "platform_limiting":
			s.Waiting++
		}
	}
	return s
}

func isActiveState(state string) bool {
	return state == "running" || state == "canceling" || state == "timing_out"
}
