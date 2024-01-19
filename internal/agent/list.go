package agent

import (
	"fmt"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type AgentListModel struct {
	agentList     list.Model
	agentPage     int
	agentLoader   tea.Cmd
	agentAppender func(page int) (tea.Msg, *buildkite.Response)
}

func NewAgentList(loader tea.Cmd, appender func(page int) (tea.Msg, *buildkite.Response)) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"

	return AgentListModel{
		agentList:     l,
		agentPage:     1,
		agentLoader:   loader,
		agentAppender: appender,
	}
}

func (m *AgentListModel) appendAgents(page int) tea.Cmd {
	// Increment to next page
	m.agentPage++
	// Set a status message and start the agentList's spinner
	startSpiner := m.agentList.StartSpinner()
	setStatus := m.agentList.NewStatusMessage(("Fetching more agents..."))
	// Fetch and append more agents
	appendAgents := func() tea.Msg {
		err, _ := m.agentAppender(m.agentPage)
		if err != nil {
			return err
		}
		return nil
	}
	return tea.Sequence(tea.Batch(startSpiner, appendAgents), setStatus)
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
	case tea.KeyMsg:
		switch msg.String() {
		case "down":
			lastListElement := m.agentList.Index() == len(m.agentList.Items())-1
			unfilteredState := m.agentList.FilterState() == list.Unfiltered
			if lastListElement && unfilteredState {
				return m, m.appendAgents(m.agentPage)
			}
		}
	// Custom messages
	case NewAgentItemsMsg:
		// when a new page of agents is received, append them to existing agents in the list and stop the loading
		// spinner
		allItems := append(m.agentList.Items(), msg.Items()...)
		cmds = append(cmds, m.agentList.SetItems(allItems))
		m.agentList.StopSpinner()
	case NewAgentAppendItemsMsg:
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
