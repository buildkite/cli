package artifact

import (
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/lipgloss"
)

func ArtifactSummary(artifact *buildkite.Artifact) string {
	artifactSummary := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(),
		lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(30).Align(lipgloss.Left).Padding(0, 1).Render(*artifact.Filename),
			lipgloss.NewStyle().Align(lipgloss.Left).Padding(0, 1).Render(FormatBytes(*artifact.FileSize)),
		),
	)

	return artifactSummary
}
