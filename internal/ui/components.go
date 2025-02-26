package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

// RenderBuildNumber formats a build number with appropriate styling
func RenderBuildNumber(state string, number int) string {
	style := StatusStyle(state)
	return style.Render(fmt.Sprintf("Build #%d", number))
}

// RenderBuildSummary renders a summary of a build
func RenderBuildSummary(b *buildkite.Build) string {
	message := TrimMessage(b.Message)
	buildInfo := fmt.Sprintf("%s %s %s",
		RenderBuildNumber(b.State, b.Number),
		message,
		RenderStatus(b.State, WithBlocked(b.Blocked)),
	)

	// Format the creator information
	var creator string
	if b.Creator.ID != "" {
		creator = b.Creator.Name
	} else if b.Author.Username != "" {
		creator = b.Author.Name
	} else {
		creator = "Unknown"
	}

	source := fmt.Sprintf("Triggered via %s by %s ∘ Created on %s",
		b.Source,
		creator,
		FormatDate(b.CreatedAt.Time),
	)

	// Format the commit information
	hash := b.Commit
	if len(hash) > 0 {
		hash = hash[0:]
	}
	commitDetails := fmt.Sprintf("Branch: %s / Commit: %s", b.Branch, hash)

	return lipgloss.JoinVertical(lipgloss.Top,
		Bold.Copy().Padding(0, 1).Render(buildInfo),
		Padding.Render(source),
		Padding.Render(commitDetails),
	)
}

// RenderJobSummary renders a summary of a job
func RenderJobSummary(job buildkite.Job) string {
	jobState := RenderStatus(job.State)
	
	// Get the job name
	var jobName string
	switch {
	case job.Name != "":
		jobName = job.Name
	case job.Label != "":
		jobName = job.Label
	default:
		jobName = job.Command
	}
	
	// Calculate duration
	var jobDuration time.Duration
	if job.Type == "script" && job.StartedAt != nil && job.FinishedAt != nil {
		jobDuration = job.FinishedAt.Time.Sub(job.StartedAt.Time)
	}
	
	// Render the duration with grey color
	durationStr := ""
	if jobDuration > 0 {
		durationStr = Faint.Render(FormatDuration(jobDuration))
	}
	
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 1).Render(""),
		Row(Bold.Render(jobState), jobName, durationStr),
	)
}

// RenderAnnotation renders a build annotation
func RenderAnnotation(annotation *buildkite.Annotation) string {
	style := StatusStyle(annotation.Style)
	icon := StatusIcon(annotation.Style)
	
	body := TruncateAndStripTags(annotation.BodyHTML, MaxPreviewLength)
	
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 1).Render(""),
		BorderRounded.Copy().BorderForeground(style.GetForeground()).
			Render(fmt.Sprintf("%s\t%s", icon, body)),
	)
}

// RenderArtifact renders a build artifact
func RenderArtifact(artifact *buildkite.Artifact) string {
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 1).Render(""),
		Row(
			lipgloss.NewStyle().Width(38).Render(artifact.ID),
			lipgloss.NewStyle().Width(50).Render(artifact.Path),
			lipgloss.NewStyle().Render(FormatBytes(artifact.FileSize)),
		),
	)
}

// RenderAgentSummary renders a summary of an agent
func RenderAgentSummary(agent buildkite.Agent) string {
	// Get queue name from metadata
	var queue string
	for _, m := range agent.Metadata {
		if queueParts := strings.Split(m, "queue="); len(queueParts) > 1 {
			queue = queueParts[1]
			break
		}
	}
	if queue == "" {
		queue = "default"
	}
	
	// Style the agent state
	stateStyle := lipgloss.NewStyle().Bold(true)
	if agent.ConnectedState == "connected" {
		stateStyle = stateStyle.Foreground(ColorSuccess)
	} else {
		stateStyle = stateStyle.Foreground(ColorPending)
	}
	
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 1).Render(""),
		Row(
			stateStyle.Render(agent.ConnectedState),
			Bold.Render(agent.Name),
			Faint.Render(agent.Version),
			lipgloss.NewStyle().Foreground(ColorInfo).Render(queue),
		),
	)
}

// RenderClusterSummary renders a summary of a cluster
func RenderClusterSummary(cluster buildkite.Cluster) string {
	var rows [][]string
	rows = append(rows, []string{cluster.Name, cluster.ID, cluster.DefaultQueueID})
	
	return Table([]string{"Name", "ID", "Default Queue ID"}, rows)
}
