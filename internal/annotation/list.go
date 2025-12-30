package annotation

import (
	"strings"

	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// AnnotationSummary renders a summary of a build annotation
func AnnotationSummary(annotation *buildkite.Annotation) string {
	if annotation == nil {
		return ""
	}

	body := StripTags(annotation.BodyHTML)
	const maxBody = 160
	bodyRunes := []rune(body)
	if len(bodyRunes) > maxBody {
		body = string(bodyRunes[:maxBody]) + "..."
	}

	rows := [][]string{
		{"Style", output.ValueOrDash(annotation.Style)},
		{"Context", output.ValueOrDash(annotation.Context)},
		{"Body", output.ValueOrDash(strings.TrimSpace(body))},
	}

	return output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)
}
