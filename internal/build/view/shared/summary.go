package shared

import (
	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/go-buildkite/v4"
)

// BuildSummary renders a build summary that can be used by multiple commands
func BuildSummary(b *buildkite.Build) string {
	return ui.RenderBuildSummary(b)
}

// BuildSummaryWithJobs renders a build summary with jobs, used by watch command
func BuildSummaryWithJobs(b *buildkite.Build) string {
	// Start with the basic build summary
	summary := BuildSummary(b)

	// Add jobs section if jobs are present
	if len(b.Jobs) > 0 {
		jobsSection := ui.Section("Jobs", renderJobs(b.Jobs))
		summary += "\n\n" + jobsSection
	}

	return summary
}

// renderJobs renders all jobs in a build
func renderJobs(jobs []buildkite.Job) string {
	var jobSections []string

	for _, j := range jobs {
		if j.Type == "script" {
			jobSections = append(jobSections, ui.RenderJobSummary(j))
		}
	}

	return ui.SpacedVertical(jobSections...)
}

// RenderJobSummary renders a job summary that can be used by multiple commands
func RenderJobSummary(j buildkite.Job) string {
	return ui.RenderJobSummary(j)
}
