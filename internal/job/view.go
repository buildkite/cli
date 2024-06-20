package job

import (
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/lipgloss"
)

type Job buildkite.Job

func JobSummary(job Job) string {
	return job.Summarise()
}

func (j Job) Summarise() string {
	var summary string
	var jobState string
	var jobName string
	var jobDuration time.Duration

	jobState = renderJobState(*j.State)

	if j.getJobType() != "script" {
		jobName = j.getJobType()
		jobDuration = time.Duration(0)
	} else {
		jobName = j.getJobName()
		jobDuration = calculateTotalTime(j.StartedAt, j.FinishedAt)
	}

	summary = lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(jobState),
			lipgloss.NewStyle().Padding(0, 1).Render(jobName),
			lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#6c6c6c")).Render(jobDuration.String()),
		),
	)

	return summary
}

func (j Job) getJobType() string {
	return *j.Type
}

func (j Job) getJobName() string {
	var jobName string

	switch {
	case j.Name != nil:
		jobName = *j.Name
	case j.Label != nil:
		jobName = *j.Label
	default:
		jobName = *j.Command
	}

	return jobName
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
	var stateColor lipgloss.Color
	switch state {
	case "passed":
		stateColor = lipgloss.Color("#9dcc3a") // green
	case "running":
		stateColor = lipgloss.Color("#FF6E00")
	case "failed", "failing":
		stateColor = lipgloss.Color("#ff0000")
	default:
		stateColor = lipgloss.Color("#5A5A5A") // grey
	}
	return stateColor
}

func calculateTotalTime(startedAt, finishedAt *buildkite.Timestamp) time.Duration {
	if startedAt == nil || finishedAt == nil {
		return 0
	}

	return finishedAt.Time.Sub(startedAt.Time)
}
