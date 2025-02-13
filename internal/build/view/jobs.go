package view

import (
	"time"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

func (v *BuildView) renderJobs() string {
	var sections []string

	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("Jobs")
	sections = append(sections, title)

	for _, j := range v.Build.Jobs {
		if j.Type == "script" {
			sections = append(sections, renderJob(j))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

func renderJob(job buildkite.Job) string {
	var jobState string
	var jobName string
	var jobDuration time.Duration

	jobState = renderJobState(job.State)

	if job.Type != "script" {
		jobName = job.Type
		jobDuration = time.Duration(0)
	} else {
		jobName = getJobName(job)
		jobDuration = calculateJobDuration(job)
	}

	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(""),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(jobState),
			lipgloss.NewStyle().Padding(0, 1).Render(jobName),
			lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#6c6c6c")).Render(jobDuration.String()),
		),
	)
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

func calculateJobDuration(job buildkite.Job) time.Duration {
	if job.StartedAt == nil || job.FinishedAt == nil {
		return 0
	}
	return job.FinishedAt.Time.Sub(job.StartedAt.Time)
}
