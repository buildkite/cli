package artifact

import (
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

func ArtifactSummary(artifact *buildkite.Artifact) string {
	artifactSummary := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(60).Align(lipgloss.Left).Padding(0, 1).Render(artifact.Path),
			lipgloss.NewStyle().Align(lipgloss.Right).Padding(0, 1).Render(FormatBytes(artifact.FileSize)),
		),
	)

	return artifactSummary
}
