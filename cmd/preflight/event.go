package preflight

import (
	"encoding/json"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// EventType identifies the kind of preflight event.
type EventType string

const (
	EventOperation      EventType = "operation"
	EventBuildStatus    EventType = "build_status"
	EventJobFailure     EventType = "job_failure"
	EventJobRetryPassed EventType = "job_retry_passed"
	EventBuildSummary   EventType = "preflight_summary"
	EventTestFailure    EventType = "test_failure"
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

	// Incomplete is set for preflight_summary events when the CLI stops before a terminal build state.
	Incomplete bool `json:"incomplete,omitempty"`

	// StopReason describes why the summary was emitted early.
	StopReason string `json:"stop_reason,omitempty"`

	// BuildCanceled is set when the CLI attempted early-exit cleanup that cancels the remote build.
	BuildCanceled *bool `json:"build_canceled,omitempty"`

	Jobs *watch.JobSummary `json:"jobs,omitempty"`

	// Job is set for job_failure and job_retry_passed events.
	Job *buildkite.Job `json:"job,omitempty"`

	// FailedJobs is set for preflight_summary events when the build failed. Contains hard-failed jobs only (soft failures excluded).
	FailedJobs []buildkite.Job `json:"failed_jobs,omitempty"`

	// PassedJobs is set for preflight_summary events when the build passed and has 10 or fewer jobs.
	PassedJobs []buildkite.Job `json:"passed_jobs,omitempty"`

	// Duration is set for preflight_summary events. Total elapsed time of the preflight run.
	Duration time.Duration `json:"duration_ns,omitempty"`

	// TestFailures is set for test_failure events.
	TestFailures []buildkite.BuildTest `json:"test_failures,omitempty"`

	// Tests is set for preflight_summary events when aggregated test summary data is available.
	Tests internalpreflight.SummaryTests `json:"tests,omitempty"`
}

// jsonJob is the compact job shape emitted in JSON mode. The Buildkite REST
// job model contains large nested agent, cluster, priority, step, and timing
// payloads that are useful for API clients but expensive and noisy for LLM
// preflight consumers.
type jsonJob struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	Command        string `json:"command,omitempty"`
	State          string `json:"state,omitempty"`
	ExitStatus     *int   `json:"exit_status,omitempty"`
	SoftFailed     bool   `json:"soft_failed"`
	Retried        bool   `json:"retried"`
	RetriesCount   int    `json:"retries_count,omitempty"`
	RetriedInJobID string `json:"retried_in_job_id,omitempty"`
}

func newJSONJob(j buildkite.Job) jsonJob {
	return jsonJob{
		ID:             j.ID,
		Name:           j.Name,
		Command:        j.Command,
		State:          j.State,
		ExitStatus:     j.ExitStatus,
		SoftFailed:     j.SoftFailed,
		Retried:        j.Retried,
		RetriesCount:   j.RetriesCount,
		RetriedInJobID: j.RetriedInJobID,
	}
}

func newJSONJobs(jobs []buildkite.Job) []jsonJob {
	if len(jobs) == 0 {
		return nil
	}

	result := make([]jsonJob, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, newJSONJob(job))
	}
	return result
}

func (e Event) MarshalJSON() ([]byte, error) {
	type eventJSON struct {
		Type EventType `json:"type"`
		Time time.Time `json:"timestamp"`

		PreflightID string `json:"preflight_id,omitempty"`
		Title       string `json:"title,omitempty"`
		Detail      string `json:"detail,omitempty"`

		Pipeline    string `json:"pipeline,omitempty"`
		BuildNumber int    `json:"build_number,omitempty"`
		BuildURL    string `json:"build_url,omitempty"`
		BuildState  string `json:"build_state,omitempty"`

		Incomplete    bool   `json:"incomplete,omitempty"`
		StopReason    string `json:"stop_reason,omitempty"`
		BuildCanceled *bool  `json:"build_canceled,omitempty"`

		Jobs *watch.JobSummary `json:"jobs,omitempty"`

		Job *jsonJob `json:"job,omitempty"`

		FailedJobs []jsonJob `json:"failed_jobs,omitempty"`
		PassedJobs []jsonJob `json:"passed_jobs,omitempty"`

		Duration time.Duration `json:"duration_ns,omitempty"`

		TestFailures []buildkite.BuildTest           `json:"test_failures,omitempty"`
		Tests        *internalpreflight.SummaryTests `json:"tests,omitempty"`
	}

	var job *jsonJob
	if e.Job != nil {
		j := newJSONJob(*e.Job)
		job = &j
	}

	var tests *internalpreflight.SummaryTests
	if len(e.Tests.Runs) > 0 || len(e.Tests.Failures) > 0 {
		tests = &e.Tests
	}

	return json.Marshal(eventJSON{
		Type:          e.Type,
		Time:          e.Time,
		PreflightID:   e.PreflightID,
		Title:         e.Title,
		Detail:        e.Detail,
		Pipeline:      e.Pipeline,
		BuildNumber:   e.BuildNumber,
		BuildURL:      e.BuildURL,
		BuildState:    e.BuildState,
		Incomplete:    e.Incomplete,
		StopReason:    e.StopReason,
		BuildCanceled: e.BuildCanceled,
		Jobs:          e.Jobs,
		Job:           job,
		FailedJobs:    newJSONJobs(e.FailedJobs),
		PassedJobs:    newJSONJobs(e.PassedJobs),
		Duration:      e.Duration,
		TestFailures:  e.TestFailures,
		Tests:         tests,
	})
}

func newBuildSummaryEvent(preflightID, pipeline string, buildNumber int, buildURL string, finalBuild buildkite.Build, startedAt time.Time) Event {
	return Event{
		Type:        EventBuildSummary,
		Time:        time.Now(),
		PreflightID: preflightID,
		Pipeline:    pipeline,
		BuildNumber: buildNumber,
		BuildURL:    buildURL,
		BuildState:  finalBuild.State,
		Duration:    time.Since(startedAt),
	}
}

func (e *Event) ApplySummaryMeta(meta summaryMeta) {
	e.Incomplete = meta.Incomplete
	e.StopReason = meta.StopReason

	if meta.StopReason == "" {
		return
	}

	buildCanceled := meta.BuildCanceled
	e.BuildCanceled = &buildCanceled
}

func (e *Event) ApplyJobResults(finalBuild buildkite.Build, tracker *watch.JobTracker) {
	if NewResult(finalBuild).Passed() {
		if passed := tracker.PassedJobs(); len(passed) <= 10 {
			e.PassedJobs = passed
		}
		return
	}

	e.FailedJobs = tracker.FailedJobs()
}
