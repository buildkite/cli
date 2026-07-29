package build

import (
	"io"
	"strings"

	"github.com/buildkite/cli/v3/internal/build/view"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type buildSummary struct {
	ID           string               `json:"id" yaml:"id"`
	Number       int                  `json:"number" yaml:"number"`
	State        string               `json:"state" yaml:"state"`
	Message      string               `json:"message" yaml:"message"`
	Branch       string               `json:"branch" yaml:"branch"`
	Commit       string               `json:"commit" yaml:"commit"`
	WebURL       string               `json:"web_url" yaml:"web_url"`
	CreatedAt    *buildkite.Timestamp `json:"created_at" yaml:"created_at"`
	StartedAt    *buildkite.Timestamp `json:"started_at" yaml:"started_at"`
	FinishedAt   *buildkite.Timestamp `json:"finished_at" yaml:"finished_at"`
	Source       string               `json:"source" yaml:"source"`
	Organization string               `json:"organization,omitempty" yaml:"organization,omitempty"`
	Pipeline     string               `json:"pipeline,omitempty" yaml:"pipeline,omitempty"`
}

func newBuildSummary(build buildkite.Build, organization, pipeline string) buildSummary {
	return buildSummary{
		ID:           build.ID,
		Number:       build.Number,
		State:        build.State,
		Message:      build.Message,
		Branch:       build.Branch,
		Commit:       build.Commit,
		WebURL:       build.WebURL,
		CreatedAt:    build.CreatedAt,
		StartedAt:    build.StartedAt,
		FinishedAt:   build.FinishedAt,
		Source:       build.Source,
		Organization: organization,
		Pipeline:     pipeline,
	}
}

func newBuildSummaryOutput(build buildkite.Build, organization, pipeline string) output.Viewable[buildSummary] {
	textBuild := build
	textBuild.Message = singleLineBuildMessage(build.Message)
	return output.Viewable[buildSummary]{
		Data: newBuildSummary(build, organization, pipeline),
		Render: func(buildSummary) string {
			return view.BuildSummary(&textBuild, organization, pipeline)
		},
	}
}

func displayBuildSummaries(builds []buildkite.Build, organization, pipeline string, format output.Format, writer io.Writer) error {
	if format == output.FormatText {
		return displaySummaryTable(builds, writer)
	}

	summaries := make([]buildSummary, len(builds))
	for i, build := range builds {
		summaries[i] = newBuildSummary(build, organization, pipeline)
	}
	return output.Write(writer, summaries, format)
}

func singleLineBuildMessage(message string) string {
	firstLine, _, _ := strings.Cut(message, "\n")
	return firstLine
}
