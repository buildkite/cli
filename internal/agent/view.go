package agent

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/pkg/style"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func AgentDataTable(agent buildkite.Agent) string {
	// Parse metadata and queue name from returned REST API Metadata list
	metadata, queue := ParseMetadata(agent.Metadata)

	tableOut := &bytes.Buffer{}
	bold := lipgloss.NewStyle().Bold(true)
	agentStateStyle := lipgloss.NewStyle().Bold(true).Foreground(MapStatusToColour(agent.ConnectedState))
	queueStyle := lipgloss.NewStyle().Foreground(style.Teal)
	versionStyle := lipgloss.NewStyle().Foreground(style.Grey)

	fmt.Fprint(tableOut, bold.Render(agent.Name))

	t := table.New().Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().PaddingRight(2)
	})

	// Construct table row data
	t.Row("ID", agent.ID)
	t.Row("State", agentStateStyle.Render(agent.ConnectedState))
	t.Row("Queue", queueStyle.Render(queue))
	t.Row("Version", versionStyle.Render(agent.Version))
	t.Row("Hostname", agent.Hostname)
	// t.Row("PID", agent.)
	t.Row("User Agent", agent.UserAgent)
	t.Row("IP Address", agent.IPAddress)
	// t.Row("OS", agent.)
	t.Row("Connected", agent.CreatedAt.UTC().Format(time.RFC1123Z))
	// t.Row("Stopped By", agent.CreatedAt)
	t.Row("Metadata", metadata)

	fmt.Fprint(tableOut, t.Render())
	return tableOut.String()
}

func ParseMetadata(metadataList []string) (string, string) {
	var metadata, queue string

	// If no tags/only queue name (or default) is set - return a tilde (~) representing
	// no metadata key/value tags, along with the found queue name
	if len(metadataList) == 1 {
		return "~", strings.Split(metadataList[0], "=")[1]
	} else {
		// We can't guarantee order of metadata key/value pairs, extract each pair
		// and the queue name when found in the respective element string
		for _, v := range metadataList {
			if strings.Contains(v, "queue=") {
				queue = strings.Split(v, "=")[1]
			} else {
				metadata += fmt.Sprintf("%s\n", v)
			}
		}
		return metadata, queue
	}
}
