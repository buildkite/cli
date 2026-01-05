package artifact

import (
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// ArtifactSummary renders a summary of a build artifact
func ArtifactSummary(artifact *buildkite.Artifact) string {
	if artifact == nil {
		return ""
	}

	rows := [][]string{{artifact.ID, artifact.Path, FormatBytes(artifact.FileSize)}}

	return output.Table(
		[]string{"ID", "Path", "Size"},
		rows,
		map[string]string{"id": "dim", "path": "bold", "size": "dim"},
	)
}
