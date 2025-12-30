package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
	Build        *buildkite.Build
	Artifacts    []buildkite.Artifact
	Annotations  []buildkite.Annotation
	Organization string
	Pipeline     string
}

// NewBuildView creates a new BuildView instance
func NewBuildView(build *buildkite.Build, artifacts []buildkite.Artifact, annotations []buildkite.Annotation, organization, pipeline string) *BuildView {
	return &BuildView{
		Build:        build,
		Artifacts:    artifacts,
		Annotations:  annotations,
		Organization: organization,
		Pipeline:     pipeline,
	}
}

func BuildSummary(b *buildkite.Build, organization, pipeline string) string {
	return buildSummary(b, organization, pipeline)
}

func BuildSummaryWithJobs(b *buildkite.Build, organization, pipeline string) string {
	var sb strings.Builder
	sb.WriteString(buildSummary(b, organization, pipeline))

	if jobs := renderJobs(b.Jobs); jobs != "" {
		sb.WriteString("\n\n")
		sb.WriteString(jobs)
	}

	return sb.String()
}

// RenderJobSummary renders a single job's summary
func RenderJobSummary(j buildkite.Job) string {
	return renderJobs([]buildkite.Job{j})
}

// Render returns the complete build view
func (v *BuildView) Render() string {
	var sb strings.Builder

	sb.WriteString(buildSummary(v.Build, v.Organization, v.Pipeline))

	if jobs := renderJobs(v.Build.Jobs); jobs != "" {
		sb.WriteString("\n\n")
		sb.WriteString(jobs)
	}

	// Add artifacts if present

	if artifacts := renderArtifacts(v.Artifacts); artifacts != "" {
		sb.WriteString("\n\n")
		sb.WriteString(artifacts)
	}

	if annotations := renderAnnotations(v.Annotations); annotations != "" {
		sb.WriteString("\n\n")
		sb.WriteString(annotations)
	}

	return sb.String()
}

func buildSummary(b *buildkite.Build, organization, pipeline string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Build %s/%s #%d (%s)\n\n", output.ValueOrDash(organization), output.ValueOrDash(pipeline), b.Number, b.State)

	summary := output.Table(
		[]string{"Field", "Value"},
		[][]string{
			{"Message", output.ValueOrDash(truncateText(b.Message, 140))},
			{"Source", output.ValueOrDash(b.Source)},
			{"Creator", creatorName(b)},
			{"Branch", output.ValueOrDash(b.Branch)},
			{"Commit", shortenCommit(b.Commit)},
			{"URL", output.ValueOrDash(b.WebURL)},
		},
		map[string]string{"field": "bold", "value": "dim"},
	)

	sb.WriteString(summary)

	return sb.String()
}

func renderJobs(jobs []buildkite.Job) string {
	scriptJobs := filterScriptJobs(jobs)
	if len(scriptJobs) == 0 {
		return ""
	}

	headers := []string{"State", "Name", "Duration"}
	rows := make([][]string, 0, len(scriptJobs))
	for _, job := range scriptJobs {
		name := job.Name
		if name == "" {
			name = job.Label
		}
		if name == "" {
			parts := strings.Split(job.Command, "\n")
			if len(parts) > 0 {
				name = parts[0]
			}
		}
		if name == "" {
			name = "-"
		}
		name = truncateText(name, 72)

		rows = append(rows, []string{
			job.State,
			name,
			formatJobDuration(job),
		})
	}

	table := output.Table(headers, rows, map[string]string{"state": "bold", "name": "italic", "duration": "dim"})

	return fmt.Sprintf("Jobs (%d)\n\n%s", len(scriptJobs), table)
}

func renderArtifacts(artifacts []buildkite.Artifact) string {
	if len(artifacts) == 0 {
		return ""
	}

	headers := []string{"ID", "Path", "Size"}
	rows := make([][]string, 0, len(artifacts))
	for _, a := range artifacts {
		size := artifact.FormatBytes(a.FileSize)
		rows = append(rows, []string{a.ID, a.Path, size})
	}

	table := output.Table(headers, rows, map[string]string{"id": "dim", "path": "bold", "size": "dim"})

	return fmt.Sprintf("Artifacts (%d)\n\n%s", len(artifacts), table)
}

func renderAnnotations(annotations []buildkite.Annotation) string {
	if len(annotations) == 0 {
		return ""
	}

	headers := []string{"Style", "Context"}
	rows := make([][]string, 0, len(annotations))
	for _, ann := range annotations {
		rows = append(rows, []string{ann.Style, ann.Context})
	}

	table := output.Table(headers, rows, map[string]string{"style": "bold", "context": "italic"})

	return fmt.Sprintf("Annotations (%d)\n\n%s", len(annotations), table)
}

func filterScriptJobs(jobs []buildkite.Job) []buildkite.Job {
	result := make([]buildkite.Job, 0, len(jobs))
	for _, job := range jobs {
		if job.Type == "script" {
			result = append(result, job)
		}
	}
	return result
}

func creatorName(build *buildkite.Build) string {
	if build.Creator.ID != "" {
		return build.Creator.Name
	}
	if build.Author.Username != "" {
		return build.Author.Name
	}
	return "Unknown"
}

func formatJobDuration(job buildkite.Job) string {
	if job.StartedAt == nil {
		return "-"
	}
	if job.FinishedAt != nil {
		d := job.FinishedAt.Sub(job.StartedAt.Time)
		return formatDuration(d)
	}
	return formatDuration(time.Since(job.StartedAt.Time)) + " (running)"
}

const ellipsis = "…"

func truncateText(text string, maxLength int) string {
	runes := []rune(text)
	if len(runes) <= maxLength {
		return string(runes)
	}
	return string(runes[:maxLength]) + ellipsis
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func shortenCommit(commit string) string {
	if strings.TrimSpace(commit) == "" {
		return "-"
	}
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}
