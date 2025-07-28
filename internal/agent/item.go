package agent

import (
	"strings"

	"github.com/buildkite/cli/v3/internal/ui"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

type AgentListItem struct {
	buildkite.Agent
}

func (ali AgentListItem) FilterValue() string {
	return strings.Join([]string{ali.Name, ali.QueueName(), ali.ConnectedState, ali.Version}, " ")
}

func (ali AgentListItem) QueueName() string {
	for _, m := range ali.Metadata {
		if strings.Contains(m, "queue=") {
			return strings.Split(m, "=")[1]
		}
	}
	return "default"
}

func (ali AgentListItem) Title() string {
	name := ui.TruncateText(ali.Name, 20)
	queue := ui.TruncateText(ali.QueueName(), 12)

	var coloredStatus string
	if ali.Job != nil {
		coloredStatus = ui.StatusStyle(ali.Job.State).Render(ali.Job.State)
	} else {
		coloredStatus = lipgloss.NewStyle().Foreground(ui.ColorInfo).Render(ali.ConnectedState)
	}

	return name + " • " + coloredStatus + " • v" + ali.Version + " • " + queue
}

func (ali AgentListItem) Description() string {
	return "" // Not displayed when ShowDescription = false
}
