package agent

import (
	"github.com/charmbracelet/bubbles/list"
)

type NewAgentItemsMsg struct {
	Items    []AgentListItem
	NextPage int
	LastPage int
}

func (a NewAgentItemsMsg) ListItems() []list.Item {
	agg := make([]list.Item, len(a.Items))
	for i, v := range a.Items {
		agg[i] = v
	}
	return agg
}
