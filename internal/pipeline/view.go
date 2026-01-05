package pipeline

import (
	"fmt"
	"io"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/output"
)

func RenderPipeline(out io.StringWriter, p graphql.GetPipelinePipeline) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Displaying pipeline: %s\n\n", p.Name)

	rows := [][]string{}
	rows = append(rows, []string{"Favorited", fmt.Sprintf("%t", p.Favorite)})
	if p.Description != nil {
		rows = append(rows, []string{"Description", *p.Description})
	}
	if len(p.Tags) > 0 {
		tagNames := make([]string, len(p.Tags))
		for i, tag := range p.Tags {
			tagNames[i] = tag.Label
		}
		rows = append(rows, []string{"Tags", strings.Join(tagNames, ", ")})
	}

	if len(rows) > 0 {
		sb.WriteString(output.Table(
			[]string{"FIELD", "VALUE"},
			rows,
			map[string]string{"field": "dim", "value": "italic"},
		))
	}

	if metrics := renderMetrics(&p); metrics != "" {
		sb.WriteString("\n")
		sb.WriteString(metrics)
	}

	if p.Steps != nil && p.Steps.Yaml != nil {
		sb.WriteString("\n\n")
		sb.WriteString(*p.Steps.Yaml)
	}

	_, _ = out.WriteString(sb.String())
	return nil
}

func renderMetrics(p *graphql.GetPipelinePipeline) string {
	if p.Metrics == nil || len(p.Metrics.Edges) == 0 {
		return ""
	}

	rows := [][]string{}
	for _, metric := range p.Metrics.Edges {
		if metric != nil && metric.Node != nil && metric.Node.Value != nil {
			value := *metric.Node.Value
			rows = append(rows, []string{metric.Node.Label, value})
		}
	}

	if len(rows) == 0 {
		return ""
	}

	return output.Table(
		[]string{"METRIC", "VALUE"},
		rows,
		map[string]string{"metric": "dim", "value": "bold"},
	)
}
