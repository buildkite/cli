package agent

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/cli/v3/pkg/style"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// AgentDataTable renders detailed agent information in a table format
func AgentDataTable(agent buildkite.Agent) string {
	// Parse metadata and queue name from returned REST API Metadata list
	metadata, queue := ParseMetadata(agent.Metadata)

	// Create table with agent details - truncate long values
	const maxValueWidth = 40 // Max width for values

	// Apply grey styling to labels
	greyLabel := lipgloss.NewStyle().Foreground(style.Grey)

	// Style connection state to match agent list
	stateValue := lipgloss.NewStyle().Foreground(ui.ColorInfo).Render(agent.ConnectedState)

	rows := [][]string{
		{greyLabel.Render("ID"), truncateValue(agent.ID, maxValueWidth)},
		{greyLabel.Render("State"), stateValue},
		{greyLabel.Render("Queue"), truncateValue(queue, maxValueWidth)},
		{greyLabel.Render("Version"), agent.Version},
		{greyLabel.Render("Hostname"), truncateValue(agent.Hostname, maxValueWidth)},
		{greyLabel.Render("User Agent"), truncateValue(agent.UserAgent, maxValueWidth)},
		{greyLabel.Render("IP Address"), agent.IPAddress},
		{greyLabel.Render("Connected"), ui.FormatDate(agent.CreatedAt.Time)},
		{greyLabel.Render("Metadata"), truncateValue(metadata, maxValueWidth)},
	}

	// Add job information if agent has a job
	if agent.Job != nil && agent.Job.State != "" {
		jobState := agent.Job.State
		if strings.ToLower(jobState) == "running" {
			jobState = lipgloss.NewStyle().Foreground(ui.ColorRunning).Render(jobState)
		} else if strings.ToLower(jobState) == "assigned" || strings.ToLower(jobState) == "accepted" {
			jobState = lipgloss.NewStyle().Foreground(ui.ColorInfo).Render(jobState)
		}

		rows = append(rows,
			[]string{greyLabel.Render("Job State"), jobState},
		)

		if agent.Job.Label != "" {
			rows = append(rows,
				[]string{greyLabel.Render("Job Label"), truncateValue(agent.Job.Label, maxValueWidth)},
			)
		} else if agent.Job.Name != "" {
			rows = append(rows,
				[]string{greyLabel.Render("Job Name"), truncateValue(agent.Job.Name, maxValueWidth)},
			)
		}

		if agent.Job.Command != "" {
			rows = append(rows,
				[]string{greyLabel.Render("Job Command"), truncateValue(agent.Job.Command, maxValueWidth)},
			)
		}

		if agent.Job.StartedAt != nil && !agent.Job.StartedAt.IsZero() {
			rows = append(rows,
				[]string{greyLabel.Render("Job Started"), ui.FormatDate(agent.Job.StartedAt.Time)},
			)
		}
	}

	// Render agent name as a title with purple color like selected rows
	title := lipgloss.NewStyle().Bold(true).Foreground(style.Purple).Render(agent.Name)

	// Render the table with agent details (no headers)
	table := ui.Table(rows)

	return ui.SpacedVertical(title, table)
}

// truncateValue truncates a value with ellipsis if it exceeds maxWidth
func truncateValue(value string, maxWidth int) string {
	if len(value) <= maxWidth {
		return value
	}
	return truncate.StringWithTail(value, uint(maxWidth), style.Ellipsis)
}

// ParseMetadata parses agent metadata to extract queue and other metadata
func ParseMetadata(metadataList []string) (string, string) {
	var metadata, queue string

	// If no tags/only queue name (or default) is set - return a tilde (~) representing
	// no metadata key/value tags, along with the found queue name
	if len(metadataList) == 1 {
		return "~", parseQueue(metadataList[0])
	} else {
		// We can't guarantee order of metadata key/value pairs, extract each pair
		// and the queue name when found in the respective element string
		for _, v := range metadataList {
			if queueValue := parseQueue(v); queueValue != "" {
				queue = queueValue
			} else {
				metadata += v + "\n"
			}
		}
		return metadata, queue
	}
}

// parseQueue extracts queue value from "queue=value" format
func parseQueue(metadata string) string {
	parts := strings.Split(metadata, "=")
	if len(parts) > 1 && parts[0] == "queue" {
		return parts[1]
	}
	return ""
}
