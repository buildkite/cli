package view

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/build/view/shared"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

// ViewOptions represents options for viewing a build
type ViewOptions struct {
	Organization string
	Pipeline     string
	BuildNumber  int
	Web          bool
}

func (o *ViewOptions) Validate() error {
	v := validation.New()
	v.AddRule("Organization", validation.Required)
	v.AddRule("Organization", validation.Slug)
	v.AddRule("Pipeline", validation.Required)
	v.AddRule("Pipeline", validation.Slug)

	return v.Validate(map[string]interface{}{
		"Organization": o.Organization,
		"Pipeline":     o.Pipeline,
	})
}

// BuildView encapsulates the build view functionality
type BuildView struct {
	Build       *buildkite.Build
	Artifacts   []buildkite.Artifact
	Annotations []buildkite.Annotation
}

// NewBuildView creates a new BuildView instance
func NewBuildView(build *buildkite.Build, artifacts []buildkite.Artifact, annotations []buildkite.Annotation) *BuildView {
	return &BuildView{
		Build:       build,
		Artifacts:   artifacts,
		Annotations: annotations,
	}
}

// Render returns the complete build view
func (v *BuildView) Render() string {
	var sections []string

	// Use shared summary rendering
	sections = append(sections, shared.BuildSummary(v.Build))

	// Add job details if present
	if len(v.Build.Jobs) > 0 {
		title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\nJobs")
		sections = append(sections, title)

		for _, j := range v.Build.Jobs {
			if j.Type == "script" {
				sections = append(sections, shared.RenderJobSummary(j))
			}
		}
	}

	// Add artifacts if present
	if len(v.Artifacts) > 0 {
		sections = append(sections, v.renderArtifacts())
	}

	// Add annotations if present
	if len(v.Annotations) > 0 {
		sections = append(sections, v.renderAnnotations())
	}

	return strings.Join(sections, "\n\n")
}
