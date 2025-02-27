package agent

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/ui"
	"github.com/buildkite/go-buildkite/v4"
)

// AgentDataTable renders detailed agent information in a table format
func AgentDataTable(agent buildkite.Agent) string {
	// Parse metadata and queue name from returned REST API Metadata list
	metadata, queue := ParseMetadata(agent.Metadata)

	// Create table with agent details
	rows := [][]string{
		{"ID", agent.ID},
		{"State", agent.ConnectedState},
		{"Queue", queue},
		{"Version", agent.Version},
		{"Hostname", agent.Hostname},
		{"User Agent", agent.UserAgent},
		{"IP Address", agent.IPAddress},
		{"Connected", ui.FormatDate(agent.CreatedAt.Time)},
		{"Metadata", metadata},
	}

	// Render agent name as a title
	title := ui.Bold.Render(agent.Name)

	// Render the table with agent details
	table := ui.Table([]string{"Property", "Value"}, rows)

	return ui.SpacedVertical(title, table)
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
