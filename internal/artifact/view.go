package artifact

import (
	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/go-buildkite/v4"
)

// ArtifactSummary renders a summary of a build artifact
func ArtifactSummary(artifact *buildkite.Artifact) string {
	return ui.RenderArtifact(artifact)
}
