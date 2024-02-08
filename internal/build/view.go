package build

import (
	"fmt"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/lipgloss"
)

func BuildSummary(build *buildkite.Build) string {

	buildInfo := fmt.Sprintf("%s %s %s", renderBuildNumber(*build.State, *build.Number), *build.Message, renderBuildState(*build.State, *build.Blocked))

	source := fmt.Sprintf("Triggered via %s by %s ∘ Created on %s",
		*build.Source,
		build.Creator.Name,
		build.CreatedAt.UTC().Format(time.RFC1123Z))
	hash := *build.Commit
	commitDetails := fmt.Sprintf("Branch: %s / Commit: %s \n", *build.Branch, hash[0:8])
	summary := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(buildInfo),
		lipgloss.NewStyle().Padding(0, 1).Render(source),
		lipgloss.NewStyle().Padding(0, 1).Render(commitDetails),
	)
	return summary
}

func getBuildStateColor(state string) lipgloss.Color {
	var stateColor lipgloss.Color
	switch state {
	case "passed":
		stateColor = lipgloss.Color("#9dcc3a") // green
	case "creating", "scheduled", "running":
		stateColor = lipgloss.Color("#FF6E00")
	case "failed", "failing":
		stateColor = lipgloss.Color("#ff0000")
	default:
		stateColor = lipgloss.Color("#5A5A5A") // grey
	}
	return stateColor
}

func renderBuildState(state string, blocked bool) string {

	var stateIcon string
	stateColor := getBuildStateColor(state)

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

	return lipgloss.NewStyle().Foreground(stateColor).Render(stateIcon)
}

func renderBuildNumber(state string, number int) string {
	stateColor := getBuildStateColor(state)

	switch state {
	case "passed":
		stateColor = lipgloss.Color("#9dcc3a") // green
	case "running":
		stateColor = lipgloss.Color("#FF6E00") // orange
	case "failed", "failing":
		stateColor = lipgloss.Color("#ff0000") // red
	}

	return lipgloss.NewStyle().Foreground(stateColor).Render(fmt.Sprintf("Build #%d", number))
}
