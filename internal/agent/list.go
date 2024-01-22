package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type AgentListModel struct {
	agentList     list.Model
	agentViewPort viewport.Model
	agentLoader   tea.Cmd
}

func NewAgentList(loader tea.Cmd) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"
	l.SetStatusBarItemName("agent", "agents")
	l.SetFilteringEnabled(false)

	v := viewport.New(80, 30)

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
		}
	}

	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:     l,
		agentViewPort: v,
		agentLoader:   loader,
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
		m.agentViewPort.SetContent("")
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
				tableContext := AgentDataTable(agent.Agent)
				m.agentViewPort.SetContent(tableContext)
			}
		case "up":
			// Clear the viewports' data
			m.agentViewPort.SetContent("")
		case "down":
			// Clear the viewports' data
			m.agentViewPort.SetContent("")
		}
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
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		agentListStyle.Render(m.agentList.View()),
		lipgloss.JoinVertical(
			lipgloss.Top,
			//	m.agentStyle.Title.Render(m.agentList.SelectedItem().FilterValue()),
			fmt.Sprintf("%4s", m.agentViewPort.View()),
		),
	)
}
