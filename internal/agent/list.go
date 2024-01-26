package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
)

type AgentStopFn func(string, bool) any

type AgentListModel struct {
	agentList        *list.Model
	agentCurrentPage int
	agentPerPage     int
	agentLastPage    int
	agentsLoading    bool
	agentLoader      func(int) tea.Cmd
	agentStopper     AgentStopFn
}

func NewAgentList(loader func(int) tea.Cmd, page, perpage int, agentStopper AgentStopFn) AgentListModel {
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

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "web")),
		}
	}

	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:        &l,
		agentCurrentPage: page,
		agentPerPage:     perpage,
		agentLoader:      loader,
		agentStopper:     agentStopper,
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

func stopAgent(m *AgentListModel, force bool) tea.Cmd {
	if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
		index := m.agentList.Index()
		// stop the agent and update the UI
		return func() tea.Msg {
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
	}
	return nil
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "s": // stop an agent gracefully
			cmds = append(cmds, stopAgent(&m, false))
		case "S": // stop an agent forcefully
			cmds = append(cmds, stopAgent(&m, true))
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
				if err := browser.OpenURL(*agent.WebURL); err != nil {
					return m, m.agentList.NewStatusMessage(fmt.Sprintf("Failed opening agent web url: %s", err.Error()))
				}
			}
		}
	case AgentStopped:
		m.agentList.StopSpinner()
	case tea.WindowSizeMsg:
		// when viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		m.agentList.SetSize(msg.Width, msg.Height)
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
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
		return m, m.agentList.NewStatusMessage(msg.Error())
	}

	agentList, cmd := m.agentList.Update(msg)
	m.agentList = &agentList
	cmds = append(cmds, cmd)

	if m, ok := msg.(Cmder); ok {
		cmds = append(cmds, m.Cmd())
	}

	return m, tea.Batch(cmds...)
}

func (m AgentListModel) View() string {
	return m.agentList.View()
}
