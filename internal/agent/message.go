package agent

import (
	"github.com/charmbracelet/bubbles/list"
)

type AgentItemsMsg struct {
	items    []AgentListItem
	lastPage int
}

func NewAgentItemsMsg(items []AgentListItem, page int) AgentItemsMsg {
	return AgentItemsMsg{items, page}
}

func (a AgentItemsMsg) ListItems() []list.Item {
	agg := make([]list.Item, len(a.items))
	for i, v := range a.items {
		agg[i] = v
	}
	return agg
}
