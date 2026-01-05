package job

import (
	"github.com/buildkite/cli/v3/internal/build/view"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
	return view.RenderJobSummary(bkJob)
}
