package annotation

import (
	"fmt"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

func AnnotationSummary(annotation *buildkite.Annotation) string {
	summary := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(),
		lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(renderAnnotationState(annotation.Style)).Render(fmt.Sprintf("%s\t%s", renderAnnotationIcon(annotation.Style), StripTags(annotation.BodyHTML))),
	)
	return summary
}

func renderAnnotationState(state string) lipgloss.Color {
	var style lipgloss.Color
	switch state {
	case "success":
		style = STYLE_SUCCESS
	case "error":
		style = STYLE_ERROR
	case "warning":
		style = STYLE_WARNING
	case "info":
		style = STYLE_INFO
	default:
		style = STYLE_DEFAULT
	}
	return style
}

func renderAnnotationIcon(state string) string {
	var icon string
	switch state {
	case "success":
		icon = "✔"
	case "error":
		icon = "✖"
	case "warning":
		icon = "⚠"
	case "info":
		icon = "ℹ"
	default:
		icon = "🗒️"
	}
	return icon
}
