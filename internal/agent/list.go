package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type AgentListModel struct {
	agentList        list.Model
	agentCurrentPage int
	agentPerPage     int
	agentLastPage    int
	agentsLoading    bool
	agentLoader      func(int) tea.Cmd
}

func NewAgentList(loader func(int) tea.Cmd, page, perpage int) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "web")),
		}
	}

	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:        l,
		agentCurrentPage: page,
		agentPerPage:     perpage,
		agentLoader:      loader,
	}
}

func (m *AgentListModel) appendAgents() tea.Cmd {
	// Set agentsLoading
	m.agentsLoading = true
	// Set a status message and start the agentList's spinner
	startSpiner := m.agentList.StartSpinner()
	statusMessage := fmt.Sprintf("Loading more agents: page %d of %d", m.agentCurrentPage, m.agentLastPage)
	setStatus := m.agentList.NewStatusMessage(statusMessage)
	// Fetch and append more agents
	appendAgents := m.agentLoader(m.agentCurrentPage)
	return tea.Sequence(tea.Batch(startSpiner, setStatus), appendAgents)
}

func (m AgentListModel) Init() tea.Cmd {
	return m.appendAgents()
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// When viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		h, v := agentListStyle.GetFrameSize()
		m.agentList.SetSize(msg.Width-h, msg.Height-v)
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
	case tea.KeyMsg:
		switch msg.String() {
		case "down":
			// Calculate last element, unfiltered and if the last agent page via the API has been reached
			lastListItem := m.agentList.Index() == len(m.agentList.Items())-1
			unfilteredState := m.agentList.FilterState() == list.Unfiltered
			if !m.agentsLoading && lastListItem && unfilteredState {
				lastPageReached := m.agentCurrentPage > m.agentLastPage
				// If down is pressed on the last agent item, list state is unfiltered and more agents are available
				// to load the API
				if !lastPageReached {
					return m, m.appendAgents()
				} else {
					// Append a status message to alert that no more agents are available to load from the API
					setStatus := m.agentList.NewStatusMessage("No more agents to load!")
					cmds = append(cmds, setStatus)
				}
			}
		case "w":
			if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
				err := browser.OpenURL(*agent.WebURL)
				return m, m.agentList.NewStatusMessage(fmt.Sprintf("Failed opening agent web url: %s", err.Error()))
			}
		}
	// Custom messages
	case AgentItemsMsg:
		// When a new page of agents is received, append them to existing agents in the list and stop the loading
		// spinner
		allItems := append(m.agentList.Items(), msg.ListItems()...)
		cmds = append(cmds, m.agentList.SetItems(allItems))
		m.agentList.StopSpinner()
		// If the message from the initial agent load, set the last page
		if m.agentCurrentPage == 1 {
			m.agentLastPage = msg.lastPage
		}
		// Increment the models' current agent page, set agentsLoading to false
		m.agentCurrentPage++
		m.agentsLoading = false
	case error:
		m.agentList.StopSpinner()
		// Show a status message for a long time
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
