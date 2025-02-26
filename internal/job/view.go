package job

import (
	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/go-buildkite/v4"
)

type Job buildkite.Job

// JobSummary renders a job summary
func JobSummary(job Job) string {
	return job.Summarise()
}

// Summarise renders a summary of the job
func (j Job) Summarise() string {
	// Convert the internal Job type back to buildkite.Job for rendering
	bkJob := buildkite.Job(j)
	return ui.RenderJobSummary(bkJob)
}

// getJobType returns the job type
func (j Job) getJobType() string {
	return j.Type
}

// getJobName returns the job name, using the first non-empty value from Name, Label, or Command
func (j Job) getJobName() string {
	var jobName string

	switch {
	case j.Name != "":
		jobName = j.Name
	case j.Label != "":
		jobName = j.Label
	default:
		jobName = j.Command
	}

	return jobName
}
