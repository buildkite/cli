package view

import (
	"fmt"
	"regexp"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

const (
	styleDefault = lipgloss.Color("#DDD")
	styleInfo    = lipgloss.Color("#337AB7")
	styleWarning = lipgloss.Color("#FF841C")
	styleError   = lipgloss.Color("#F45756")
	styleSuccess = lipgloss.Color("#2ECC40")

	// Maximum length for annotation body preview
	maxAnnotationLength = 120
)

func (v *BuildView) renderAnnotations() string {
	var sections []string

	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("Annotations")
	sections = append(sections, title)

	for _, annotation := range v.Annotations {
		sections = append(sections, renderAnnotation(&annotation))
	}

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

func renderAnnotation(annotation *buildkite.Annotation) string {
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(""),
		lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(getAnnotationStateColor(annotation.Style)).
			Render(fmt.Sprintf("%s\t%s",
				getAnnotationIcon(annotation.Style),
				truncateAndStripTags(annotation.BodyHTML),
			)),
	)
}

func getAnnotationStateColor(state string) lipgloss.Color {
	switch state {
	case "success":
		return styleSuccess
	case "error":
		return styleError
	case "warning":
		return styleWarning
	case "info":
		return styleInfo
	default:
		return styleDefault
	}
}

func getAnnotationIcon(state string) string {
	switch state {
	case "success":
		return "✔"
	case "error":
		return "✖"
	case "warning":
		return "⚠"
	case "info":
		return "ℹ"
	default:
		return "🗒️"
	}
}

func truncateAndStripTags(body string) string {
	// Strip HTML tags while preserving newlines
	cleaned := stripTags(body)

	if len(cleaned) > maxAnnotationLength {
		return cleaned[:maxAnnotationLength] + "...(more)"
	}
	return cleaned
}

func stripTags(body string) string {
	// Remove closing tags first
	re := regexp.MustCompile(`</[^>]+>`)
	body = re.ReplaceAllString(body, "")

	// Then remove opening tags
	re = regexp.MustCompile(`<[^>]*>`)
	body = re.ReplaceAllString(body, "")

	return body
}
