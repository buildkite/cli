package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type AgentStopFn func(string, bool) any
type AgentLoadFn func() any

type AgentListModel struct {
	agentList    list.Model
	agentLoader  tea.Cmd
	agentStopper AgentStopFn
}

func NewAgentList(agentLoader tea.Cmd, agentStopper AgentStopFn) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"
	l.SetStatusBarItemName("agent", "agents")
	l.SetFilteringEnabled(false)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop")),
			key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "force stop")),
		}
	}
	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:    l,
		agentLoader:  agentLoader,
		agentStopper: agentStopper,
	}
}

func (m AgentListModel) Init() tea.Cmd {
	return m.agentLoader
}

func (m *AgentListModel) stopAgent(force bool) tea.Cmd {
	if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
		index := m.agentList.Index()
		statusMessage := fmt.Sprintf("Stopping %s (gracefully)", *agent.Name)
		if force {
			statusMessage = fmt.Sprintf("Stopping %s (forcefully)", *agent.Name)
		}
		// set a status message and start the loading spinner
		setStatus := m.agentList.NewStatusMessage(statusMessage)
		startSpinner := m.agentList.StartSpinner()
		// stop the agent and update the UI
		stopAgent := func() tea.Msg {
			err := m.agentStopper(*agent.ID, force)
			if err != nil {
				return err
			}
			m.agentList.RemoveItem(index)
			m.agentList.ResetSelected()
			return AgentStopped{
				Agent: agent,
			}
		}
		return tea.Sequence(tea.Batch(startSpinner, setStatus), stopAgent)
	}
	return nil
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "s": // stop an agent gracefully
			return m, m.stopAgent(false)
		case "S": // stop an agent forcefully
			return m, m.stopAgent(true)
		}
	case AgentStopped:
		m.agentList.StopSpinner()
		return m, nil
	case tea.WindowSizeMsg:
		// when viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		m.agentList.SetSize(msg.Width, msg.Height)
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
		return m, m.agentList.NewStatusMessage(msg.Error())
	}

	var cmd tea.Cmd
	m.agentList, cmd = m.agentList.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m AgentListModel) View() string {
	return m.agentList.View()
}
