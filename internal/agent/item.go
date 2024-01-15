package agent

import (
	"strings"

	"github.com/buildkite/go-buildkite/v3/buildkite"
)

// agentListItem implements list.Item for displaying in a list
type agentListItem struct {
	*buildkite.Agent
}

func (ali agentListItem) FilterValue() string {
	return strings.Join([]string{*ali.Name, ali.QueueName(), *ali.ConnectedState, *ali.Version}, " ")
}

func (ali agentListItem) QueueName() string {
	for _, m := range ali.Metadata {
		if strings.Contains(m, "queue=") {
			return strings.Split(m, "=")[1]
		}
	}
	return ""
}
