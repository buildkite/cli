package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type AgentListModel struct {
	agentList    list.Model
	agentLoader  tea.Cmd
	agentStopper func(string, bool) error
}

func NewAgentList(agentLoader tea.Cmd, agentStopper func(string, bool) error) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"
	l.SetFilteringEnabled(false)

	return AgentListModel{
		agentList:    l,
		agentLoader:  agentLoader,
		agentStopper: agentStopper,
	}
}

func (m AgentListModel) Init() tea.Cmd {
	return m.agentLoader
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// when viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		h, v := agentListStyle.GetFrameSize()
		m.agentList.SetSize(msg.Width-h, msg.Height-v)
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
	case NewAgentItemsMsg:
		// when a new page of agents is received, append them to existing agents in the list and stop the loading
		// spinner
		allItems := append(m.agentList.Items(), msg.Items()...)
		cmds = append(cmds, m.agentList.SetItems(allItems))
		m.agentList.StopSpinner()
	case error:
		m.agentList.StopSpinner()
		// show a status message for a long time
		m.agentList.StatusMessageLifetime = time.Duration(time.Hour)
		return m, m.agentList.NewStatusMessage(fmt.Sprintf("Failed loading agents: %s", msg.Error()))
	}

	var cmd tea.Cmd
	m.agentList, cmd = m.agentList.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m AgentListModel) View() string {
	return agentListStyle.Render(m.agentList.View())
}
