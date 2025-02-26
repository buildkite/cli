package artifact

import "github.com/buildkite/cli/v3/internal/ui"

// FormatBytes formats bytes into human-readable format (KB, MB, GB, etc.)
func FormatBytes(bytes int64) string {
	return ui.FormatBytes(bytes)
}
