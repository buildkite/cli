package annotation

import (
	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/go-buildkite/v4"
)

// AnnotationSummary renders a summary of a build annotation
func AnnotationSummary(annotation *buildkite.Annotation) string {
	return ui.RenderAnnotation(annotation)
}
