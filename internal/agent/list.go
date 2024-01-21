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
	agentList        list.Model
	agentCurrentPage int
	agentPerPage     int
	agentLastPage    int
	agentLoader      func(int, int) tea.Cmd
}

func NewAgentList(loader func(int, int) tea.Cmd, page, perpage int) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"

	return AgentListModel{
		agentList:        l,
		agentCurrentPage: page,
		agentPerPage:     perpage,
		agentLoader:      loader,
	}
}

func (m *AgentListModel) appendAgents() tea.Cmd {
	// Set a status message and start the agentList's spinner
	startSpiner := m.agentList.StartSpinner()
	setStatus := m.agentList.NewStatusMessage(("Fetching more agents..."))
	// Fetch and append more agents
	appendAgents := func() tea.Msg {
		err := m.agentLoader(m.agentCurrentPage, m.agentPerPage)
		if err != nil {
			return err
		}
		return nil
	}
	return tea.Sequence(appendAgents, tea.Batch(startSpiner, setStatus))
}

func (m AgentListModel) Init() tea.Cmd {
	startSpiner := m.agentList.StartSpinner()
	setStatus := m.agentList.NewStatusMessage(("Loading agents"))
	agentInitLoader := m.agentLoader(m.agentCurrentPage, m.agentPerPage)
	return tea.Sequence(agentInitLoader, tea.Batch(startSpiner, setStatus))
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// When viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		h, v := agentListStyle.GetFrameSize()
		m.agentList.SetSize(msg.Width-h, msg.Height-v)
	case tea.KeyMsg:
		switch msg.String() {
		case "down":
			lastListElement := m.agentList.Index() == len(m.agentList.Items())-1
			unfilteredState := m.agentList.FilterState() == list.Unfiltered
			lastPageReached := m.agentCurrentPage > m.agentLastPage
			// If down is pressed on the last agent item, list state is unfiltered and more agents are available by the API
			if lastListElement && unfilteredState && !lastPageReached {
				return m, m.appendAgents()
			}
		}
	// Custom messages
	case NewAgentItemsMsg:
		// when a new page of agents is received, append them to existing agents in the list and stop the loading
		// spinner
		allItems := append(m.agentList.Items(), msg.ListItems()...)
		cmds = append(cmds, m.agentList.SetItems(allItems))
		// Stop the loading spinner
		m.agentList.StopSpinner()
		// If the message from the initial agent load, set the last page
		if m.agentCurrentPage == 1 {
			m.agentLastPage = msg.LastPage
		}
		// Update the page to the next
		m.agentCurrentPage = msg.NextPage
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
