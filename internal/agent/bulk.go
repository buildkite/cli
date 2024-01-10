package agent

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type BulkAgent struct {
	Agents []StoppableAgent
}

func (bulkAgent BulkAgent) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(bulkAgent.Agents))
	for i, agent := range bulkAgent.Agents {
		cmds[i] = agent.Init()
	}

	return tea.Batch(cmds...)
}

func (bulkAgent BulkAgent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// if a key is pressed, ignore everything except for common quitting
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return bulkAgent, tea.Quit
		default:
			return bulkAgent, nil
		}
	}
	cmds := make([]tea.Cmd, len(bulkAgent.Agents))
	for i, agent := range bulkAgent.Agents {
		agent, cmd := agent.Update(msg)
		bulkAgent.Agents[i] = agent.(StoppableAgent)
		cmds[i] = cmd
	}
	return bulkAgent, tea.Batch(cmds...)
}

func (bulkAgent BulkAgent) View() string {
	var sb strings.Builder

	for _, agent := range bulkAgent.Agents {
		sb.WriteString(agent.View())
	}

	return sb.String()
}
