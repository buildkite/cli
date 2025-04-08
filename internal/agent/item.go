package agent

import (
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// AgentListItem implements list.Item for displaying in a list
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
