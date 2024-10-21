package build

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

func BuildSummary(build buildkite.Build) string {
	message := trimMessage(build.Message)
	buildInfo := fmt.Sprintf("%s %s %s", renderBuildNumber(build.State, build.Number), message, renderBuildState(build.State, build.Blocked))

	source := fmt.Sprintf("Triggered via %s by %s ∘ Created on %s",
		build.Source,
		buildCreator(build),
		build.CreatedAt.UTC().Format(time.RFC1123Z))
	hash := build.Commit
	if len(hash) > 0 {
		hash = hash[0:]
	}
	commitDetails := fmt.Sprintf("Branch: %s / Commit: %s \n", build.Branch, hash)
	summary := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(buildInfo),
		lipgloss.NewStyle().Padding(0, 1).Render(source),
		lipgloss.NewStyle().Padding(0, 1).Render(commitDetails),
	)
	return summary
}

// buildCreator returns the creator of a build factoring in the creator/author fallback
func buildCreator(build buildkite.Build) string {
	if build.Creator.ID != "" {
		return build.Creator.Name
	}
	if build.Author.Username != "" {
		return build.Author.Name
	}

	// if we cannot return any name then just return an empty string?
	// it is possible to have no creator or author, for example a scheduled build
	return "Unknown"
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

func trimMessage(msg string) string {
	newlineIndex := strings.Index(msg, "\n")
	if newlineIndex != -1 {
		beforeNewline := msg[:newlineIndex]
		return beforeNewline + "..."
	} else {
		return msg
	}
}
