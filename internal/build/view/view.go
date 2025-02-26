package view

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/go-buildkite/v4"
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

	// Add build summary
	sections = append(sections, ui.RenderBuildSummary(v.Build))

	// Add job details if present
	if len(v.Build.Jobs) > 0 {
		jobsSection := ui.Section("Jobs", v.renderJobs())
		sections = append(sections, jobsSection)
	}

	// Add artifacts if present
	if len(v.Artifacts) > 0 {
		artifactsSection := ui.Section("Artifacts", v.renderArtifacts())
		sections = append(sections, artifactsSection)
	}

	// Add annotations if present
	if len(v.Annotations) > 0 {
		annotationsSection := ui.Section("Annotations", v.renderAnnotations())
		sections = append(sections, annotationsSection)
	}

	return ui.SpacedVertical(sections...)
}

func (v *BuildView) renderJobs() string {
	var jobSections []string

	for _, j := range v.Build.Jobs {
		if j.Type == "script" {
			jobSections = append(jobSections, ui.RenderJobSummary(j))
		}
	}

	return strings.Join(jobSections, "\n")
}

func (v *BuildView) renderArtifacts() string {
	var artifactSections []string

	for _, artifact := range v.Artifacts {
		artifactSections = append(artifactSections, ui.RenderArtifact(&artifact))
	}

	return strings.Join(artifactSections, "\n")
}

func (v *BuildView) renderAnnotations() string {
	var annotationSections []string

	for _, annotation := range v.Annotations {
		annotationSections = append(annotationSections, ui.RenderAnnotation(&annotation))
	}

	return strings.Join(annotationSections, "\n")
}
