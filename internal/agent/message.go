package agent

import (
	"github.com/charmbracelet/bubbles/list"
)

type NewAgentItemsMsg []agentListItem

func (a NewAgentItemsMsg) Items() []list.Item {
	agg := make([]list.Item, len(a))
	for i, v := range a {
		agg[i] = v
	}
	return agg
}
