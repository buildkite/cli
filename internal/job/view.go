package job

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/lipgloss"
)

func JobSummary(job *buildkite.Job) string {
	jobName := getJobName(*job)
  jobTotalTime, err := calculateTotalTime(job.StartedAt, job.FinishedAt)
  if err != nil {
    log.Fatal("Unable to calculate total job time", err)
  }

  jobInfo := fmt.Sprintf("%s %s (%s)", renderJobState(*job.State), jobName, lipgloss.NewStyle().Foreground(lipgloss.Color("#5A5A5A")).Render(jobTotalTime.String()))

  summary := lipgloss.JoinVertical(lipgloss.Top,
    lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(),
    lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(jobInfo),
    )
	return summary
}

func getJobName(job buildkite.Job) string {
	var jobName string

	switch {
	case job.Name != nil:
		jobName = *job.Name
	case job.Label != nil:
		jobName = *job.Label
	default:
		jobName = *job.Command
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

func calculateTotalTime(startedAt, finishedAt *buildkite.Timestamp) (time.Duration, error) {
    if startedAt == nil || finishedAt == nil {
        return 0, errors.New("both startedAt and finishedAt must be non-nil")
    }

    return finishedAt.Time.Sub(startedAt.Time), nil
}
