package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

// BuildSummary renders a build summary that can be used by multiple commands
func BuildSummary(b *buildkite.Build) string {
	message := trimMessage(b.Message)
	buildInfo := fmt.Sprintf("%s %s %s",
		renderBuildNumber(b.State, b.Number),
		message,
		renderBuildState(b.State, b.Blocked),
	)

	source := fmt.Sprintf("Triggered via %s by %s ∘ Created on %s",
		b.Source,
		buildCreator(b),
		b.CreatedAt.UTC().Format(time.RFC1123Z),
	)

	hash := b.Commit
	if len(hash) > 0 {
		hash = hash[0:]
	}
	commitDetails := fmt.Sprintf("Branch: %s / Commit: %s", b.Branch, hash)

	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(buildInfo),
		lipgloss.NewStyle().Padding(0, 1).Render(source),
		lipgloss.NewStyle().Padding(0, 1).Render(commitDetails),
	)
}

// BuildSummaryWithJobs renders a build summary with jobs, used by watch command
func BuildSummaryWithJobs(b *buildkite.Build) string {
	summary := BuildSummary(b)

	if len(b.Jobs) > 0 {
		summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\nJobs")
		for _, j := range b.Jobs {
			if j.Type == "script" {
				summary += RenderJobSummary(j)
			}
		}
	}

	return summary
}

func RenderJobSummary(job buildkite.Job) string {
	jobState := renderJobState(job.State)
	jobName := getJobName(job)
	jobDuration := calculateJobDuration(job)

	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(""),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(jobState),
			lipgloss.NewStyle().Padding(0, 1).Render(jobName),
			lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#6c6c6c")).Render(jobDuration.String()),
		),
	)
}

func renderJobState(state string) string {
	var stateIcon string
	stateColor := getJobStateColor(state)

	switch state {
	case "passed":
		stateIcon = "✔"
	case "running":
		stateIcon = "▶"
	case "failed", "failing":
		stateIcon = "✖"
	case "canceled":
		stateIcon = "🚫"
	case "canceling":
		stateIcon = "🚫(cancelling...)"
	case "blocked":
		stateIcon = "🔒(Blocked)"
	case "unblocked":
		stateIcon = "🔓(Unblocked)"
	default:
		stateIcon = "❔"
	}

	return lipgloss.NewStyle().Foreground(stateColor).Render(stateIcon)
}

func getJobStateColor(state string) lipgloss.Color {
	switch state {
	case "passed":
		return lipgloss.Color("#9dcc3a")
	case "running":
		return lipgloss.Color("#FF6E00")
	case "failed", "failing":
		return lipgloss.Color("#ff0000")
	default:
		return lipgloss.Color("#5A5A5A")
	}
}

func getJobName(job buildkite.Job) string {
	switch {
	case job.Name != "":
		return job.Name
	case job.Label != "":
		return job.Label
	default:
		return job.Command
	}
}

func calculateJobDuration(job buildkite.Job) time.Duration {
	if job.StartedAt == nil || job.FinishedAt == nil {
		return 0
	}
	return job.FinishedAt.Time.Sub(job.StartedAt.Time)
}

// Utility functions shared between commands
func trimMessage(msg string) string {
	if idx := strings.Index(msg, "\n"); idx != -1 {
		return msg[:idx] + "..."
	}
	return msg
}

func buildCreator(b *buildkite.Build) string {
	if b.Creator.ID != "" {
		return b.Creator.Name
	}
	if b.Author.Username != "" {
		return b.Author.Name
	}
	return "Unknown"
}

func renderBuildNumber(state string, number int) string {
	style := lipgloss.NewStyle().Foreground(getBuildStateColor(state))
	return style.Render(fmt.Sprintf("Build #%d", number))
}

func renderBuildState(state string, blocked bool) string {
	var stateIcon string
	style := lipgloss.NewStyle().Foreground(getBuildStateColor(state))

	switch state {
	case "passed":
		stateIcon = "✔"
		if blocked {
			stateIcon = "✔(blocked)"
		}
	case "running":
		stateIcon = "▶"
	case "scheduled":
		stateIcon = "⏰"
	case "failed", "failing":
		stateIcon = "✖"
	case "canceled":
		stateIcon = "🚫"
	case "canceling":
		stateIcon = "🚫(cancelling...)"
	default:
		stateIcon = "❔"
	}

	return style.Render(stateIcon)
}

func getBuildStateColor(state string) lipgloss.Color {
	switch state {
	case "passed":
		return lipgloss.Color("#9dcc3a")
	case "running", "scheduled":
		return lipgloss.Color("#FF6E00")
	case "failed", "failing":
		return lipgloss.Color("#ff0000")
	default:
		return lipgloss.Color("#5A5A5A")
	}
}
