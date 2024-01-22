package agent

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type NewAgentItemsMsg []AgentListItem

// Msger is implemented by any value that can supply a bubbletea Msg.
// The Msg method is used to pass a Msg to a running bubbletea Program.
type Msger interface {
	Msg() tea.Msg
}

// Cmder is implemented by any value that can supply a bubbletea Cmd.
// The Cmd method is used to pass a Cmd to a running bubbletea Program.
type Cmder interface {
	Cmd() tea.Cmd
}

func (a NewAgentItemsMsg) Items() []list.Item {
	agg := make([]list.Item, len(a))
	for i, v := range a {
		agg[i] = v
	}
	return agg
}

type AgentStopped struct {
	Agent AgentListItem
	cmd   tea.Cmd
}

func (a AgentStopped) Cmd() tea.Cmd {
	return a.cmd
}
