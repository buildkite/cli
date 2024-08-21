package pipeline

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/style"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func RenderPipeline(out io.StringWriter, p graphql.GetPipelinePipeline) error {
	hr := lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.ThickBorder())
	if p.Color != nil {
		hr = hr.BorderForeground(lipgloss.Color(*p.Color))
	}

	var header, tags, metrics, steps strings.Builder

	err := renderHeader(&header, &p)
	if err != nil {
		return err
	}
	err = renderTags(&tags, &p)
	if err != nil {
		return err
	}
	err = renderMetrics(&metrics, &p)
	if err != nil {
		return err
	}
	err = renderSteps(&steps, &p)
	if err != nil {
		return err
	}

	// find the longest section to use as the overall width
	w := lipgloss.Width(header.String())
	for _, section := range []string{tags.String(), metrics.String(), steps.String()} {
		if current := lipgloss.Width(section); current > w {
			w = current
		}
	}

	_, _ = out.WriteString(lipgloss.JoinVertical(
		lipgloss.Center,
		// add in 4 extra spaces to the header to accommodate the indentation of the steps
		hr.Width(w+4).AlignHorizontal(lipgloss.Center).Render(header.String()),
		tags.String(),
		metrics.String(),
	))
	_, _ = out.WriteString(steps.String())

	return nil
}

func renderHeader(header io.StringWriter, p *graphql.GetPipelinePipeline) error {
	italic := lipgloss.NewStyle().Italic(true)
	bold := lipgloss.NewStyle().Bold(true)

	if p.Emoji != nil {
		_, _ = header.WriteString(*p.Emoji)
		_, _ = header.WriteString(" ")
	}
	_, _ = header.WriteString(bold.Render(p.Name))

	if p.Description != nil {
		_, _ = header.WriteString(fmt.Sprintf(": %s", italic.Render(*p.Description)))
	}

	if p.Favorite {
		_, _ = header.WriteString(" ⭐")
	}
	return nil
}

func renderTags(tags io.StringWriter, p *graphql.GetPipelinePipeline) error {
	if numTags := len(p.Tags); numTags > 0 {
		for i, tag := range p.Tags {
			_, _ = tags.WriteString(tag.Label)
			if i < numTags-1 {
				_, _ = tags.WriteString(" ")
			}
		}
	}

	return nil
}

func renderMetrics(metrics io.StringWriter, p *graphql.GetPipelinePipeline) error {
	if p.Metrics != nil && len(p.Metrics.Edges) > 0 {
		for i, metric := range p.Metrics.Edges {
			if metric != nil && metric.Node != nil && metric.Node.Value != nil {
				value := *metric.Node.Value
				percent, _ := strconv.Atoi(strings.Trim(value, "%"))
				if metric.Node.Label == "Reliability" {
					color := style.Red
					if percent >= 90 {
						color = style.Green
					} else if percent >= 70 {
						color = style.Olive
					}
					value = lipgloss.NewStyle().Foreground(color).Render(value)
				}
				m := fmt.Sprintf("%s: %s", metric.Node.Label, value)
				_, _ = metrics.WriteString(m)

				if i < len(p.Metrics.Edges)-1 {
					_, _ = metrics.WriteString("    ")
				}
			}
		}
	}
	return nil
}

func renderSteps(steps io.StringWriter, p *graphql.GetPipelinePipeline) error {
	if p.Steps != nil && p.Steps.Yaml != nil {
		render, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithEmoji(), glamour.WithWordWrap(0))
		r, _ := render.Render(fmt.Sprintf("```yaml\n%s\n```\n", *p.Steps.Yaml))
		_, _ = steps.WriteString(r)
	}
	return nil
}
