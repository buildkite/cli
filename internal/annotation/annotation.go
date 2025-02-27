package annotation

import (
	"github.com/buildkite/cli/v3/internal/ui"
)

// StripTags removes HTML tags from a string
func StripTags(html string) string {
	return ui.StripHTMLTags(html)
}
