package shared

import (
	"github.com/buildkite/cli/v3/internal/build/view"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// BuildSummary renders a build summary that can be used by multiple commands
func BuildSummary(b *buildkite.Build, organization, pipeline string) string {
	return view.BuildSummary(b, organization, pipeline)
}

// BuildSummaryWithJobs renders a build summary with jobs, used by watch command
func BuildSummaryWithJobs(b *buildkite.Build, organization, pipeline string) string {
	return view.BuildSummaryWithJobs(b, organization, pipeline)
}

// RenderJobSummary renders a job summary that can be used by multiple commands
func RenderJobSummary(j buildkite.Job) string {
	return view.RenderJobSummary(j)
}
