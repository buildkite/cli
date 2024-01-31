package agent

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
)

type AgentStopFn func(string, bool) any

var agentListStyle = lipgloss.NewStyle().Padding(1, 2)
var viewPortStyle = agentListStyle.Copy()

type AgentListModel struct {
	agentList          *list.Model
	agentViewPort      viewport.Model
	agentDataDisplayed bool
	agentCurrentPage   int
	agentPerPage       int
	agentLastPage      int
	agentsLoading      bool
	agentLoader        func(int) tea.Cmd
	agentStopper       AgentStopFn
}

func NewAgentList(loader func(int) tea.Cmd, page, perpage int, agentStopper AgentStopFn) AgentListModel {
	l := list.New(nil, NewDelegate(), 0, 0)
	l.Title = "Buildkite Agents"
	l.SetStatusBarItemName("agent", "agents")
	l.SetFilteringEnabled(false)

	v := viewport.New(0, 0)
	v.SetContent("")

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "force stop")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop")),
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
			key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "web")),
		}
	}

	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:          &l,
		agentViewPort:      v,
		agentDataDisplayed: false,
		agentCurrentPage:   page,
		agentPerPage:       perpage,
		agentLoader:        loader,
		agentStopper:       agentStopper,
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

func (m *AgentListModel) clearAgentViewPort() {
	m.agentViewPort.SetContent("")
	m.agentDataDisplayed = false
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
	case tea.WindowSizeMsg:
		// When viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		h, v := agentListStyle.GetFrameSize()
		m.agentList.SetSize(msg.Width-h, msg.Height-v)
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
	case tea.KeyMsg:
		switch msg.String() {
		case "s": // stop an agent gracefully
			cmds = append(cmds, stopAgent(&m, false))
		case "S": // stop an agent forcefully
			cmds = append(cmds, stopAgent(&m, true))
		case "v":
			if !m.agentDataDisplayed {
				if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
					tableContext := AgentDataTable(agent.Agent)
					m.agentViewPort.SetContent(tableContext)
					m.agentDataDisplayed = true
				}
			} else {
				m.clearAgentViewPort()
			}
		case "up":
			m.clearAgentViewPort()
		case "down":
			m.clearAgentViewPort()
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
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		agentListStyle.Render(m.agentList.View()),
		lipgloss.JoinVertical(
			lipgloss.Top,
			viewPortStyle.Render(m.agentViewPort.View()),
		),
	)
}
