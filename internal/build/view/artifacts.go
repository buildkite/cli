package view

import (
	"fmt"

	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

func (v *BuildView) renderArtifacts() string {
	var sections []string

	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("Artifacts")
	sections = append(sections, title)

	for _, artifact := range v.Artifacts {
		sections = append(sections, renderArtifact(&artifact))
	}

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

func renderArtifact(artifact *buildkite.Artifact) string {
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(""),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(38).Align(lipgloss.Left).Padding(0, 1).Render(artifact.ID),
			lipgloss.NewStyle().Width(50).Align(lipgloss.Left).Padding(0, 1).Render(artifact.Path),
			lipgloss.NewStyle().Align(lipgloss.Right).Padding(0, 1).Render(formatBytes(artifact.FileSize)),
		),
	)
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
