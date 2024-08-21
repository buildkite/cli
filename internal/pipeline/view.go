package pipeline

import (
	"fmt"
	"io"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/style"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type writer interface {
	io.Writer
	WriteString(string) (int, error)
}

func RenderPipeline(out writer, p graphql.GetPipelinePipeline) error {
	hr := lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.ThickBorder())
	if p.Color != nil {
		hr = hr.BorderForeground(lipgloss.Color(*p.Color))
	}
	italic := lipgloss.NewStyle().Italic(true)
	bold := lipgloss.NewStyle().Bold(true)

	var header, tags, metrics, steps strings.Builder

	if p.Emoji != nil {
		header.WriteString(*p.Emoji)
		header.WriteString(" ")
	}
	header.WriteString(bold.Render(p.Name))

	if p.Description != nil {
		header.WriteString(fmt.Sprintf(": %s", italic.Render(*p.Description)))
	}

	if p.Favorite {
		header.WriteString(" ⭐")
	}

	if numTags := len(p.Tags); numTags > 0 {
		for i, tag := range p.Tags {
			tags.WriteString(tag.Label)
			if i < numTags-1 {
				tags.WriteString(" ")
			}
		}
	}

	if p.Metrics != nil && len(p.Metrics.Edges) > 0 {
		for i, metric := range p.Metrics.Edges {
			if metric != nil && metric.Node != nil && metric.Node.Value != nil {
				val := *metric.Node.Value
				if metric.Node.Label == "Reliability" {
					// TODO: change colour depending on percent threshold
					val = lipgloss.NewStyle().Foreground(style.Green).Render(*metric.Node.Value)
				}
				m := fmt.Sprintf("%s: %s", metric.Node.Label, val)
				metrics.WriteString(m)

				if i < len(p.Metrics.Edges)-1 {
					metrics.WriteString("    ")
				}
			}
		}
	}

	// find the longest section to use as the overall width

	if p.Steps != nil && p.Steps.Yaml != nil {
		render, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithEmoji(), glamour.WithWordWrap(0))
		r, _ := render.Render(fmt.Sprintf("```yaml\n%s\n```\n", *p.Steps.Yaml))
		steps.WriteString(r)
	}

	w := lipgloss.Width(header.String())
	for _, section := range []string{tags.String(), metrics.String(), steps.String()} {
		if current := lipgloss.Width(section); current > w {
			w = current
		}
	}

	fmt.Fprintf(out, "%s", lipgloss.JoinVertical(
		lipgloss.Center,
		// add in 4 extra spaces to the header to accommodate the indentation of the steps
		hr.Width(w+4).AlignHorizontal(lipgloss.Center).Render(header.String()),
		tags.String(),
		metrics.String(),
	))
	fmt.Fprintf(out, "%s", steps.String())

	return nil
}
