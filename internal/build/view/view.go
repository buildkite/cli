package view

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/models"
	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/go-buildkite/v4"
)

// ViewOptions represents options for viewing a build
// It uses the models.Build struct plus a web flag
type ViewOptions struct {
	models.Build
	Web bool
}

func (o *ViewOptions) Validate() error {
	v := validation.New()
	v.AddRule("Organization", validation.Required)
	v.AddRule("Organization", validation.Slug)
	v.AddRule("Pipeline", validation.Required)
	
	// Pipeline is now a struct pointer, not a string
	var pipelineSlug string
	if o.Pipeline != nil {
		pipelineSlug = o.Pipeline.Slug
	}
	
	return v.Validate(map[string]interface{}{
		"Organization": o.Organization,
		"Pipeline":     pipelineSlug,
	})
}

// BuildView encapsulates the build view functionality
type BuildView struct {
	Build       *buildkite.Build
	Artifacts   []models.Artifact
	Annotations []models.Annotation
}

// NewBuildView creates a new BuildView instance
func NewBuildView(build *buildkite.Build, artifacts []buildkite.Artifact, annotations []buildkite.Annotation) *BuildView {
	// Convert buildkite.Artifact to models.Artifact
	modelArtifacts := make([]models.Artifact, len(artifacts))
	for i, a := range artifacts {
		artifact := models.FromBuildkiteArtifact(&a)
		if artifact != nil {
			modelArtifacts[i] = *artifact
		}
	}

	// Convert buildkite.Annotation to models.Annotation
	modelAnnotations := make([]models.Annotation, len(annotations))
	for i, a := range annotations {
		annotation := models.FromBuildkiteAnnotation(&a)
		if annotation != nil {
			modelAnnotations[i] = *annotation
		}
	}

	return &BuildView{
		Build:       build,
		Artifacts:   modelArtifacts,
		Annotations: modelAnnotations,
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
		// Convert to buildkite Artifact for now using our conversion utility
		bkArtifact := artifact.ToBuildkiteArtifact()
		if bkArtifact != nil {
			artifactSections = append(artifactSections, ui.RenderArtifact(bkArtifact))
		}
	}

	return strings.Join(artifactSections, "\n")
}

func (v *BuildView) renderAnnotations() string {
	var annotationSections []string

	for _, annotation := range v.Annotations {
		// Convert to buildkite Annotation for now using our conversion utility
		bkAnnotation := annotation.ToBuildkiteAnnotation()
		if bkAnnotation != nil {
			annotationSections = append(annotationSections, ui.RenderAnnotation(bkAnnotation))
		}
	}

	return strings.Join(annotationSections, "\n")
}