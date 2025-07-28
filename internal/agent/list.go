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

var (
	agentListStyle = lipgloss.NewStyle().Padding(1, 2)
	viewPortStyle  = lipgloss.NewStyle().Padding(1, 2)
)

type AgentListModel struct {
	agentList          list.Model
	agentViewPort      viewport.Model
	agentDataDisplayed bool
	agentCurrentPage   int
	agentPerPage       int
	agentLastPage      int
	agentsLoading      bool
	agentLoader        func(int) tea.Cmd
	terminalWidth      int
	terminalHeight     int
	isRefreshing       bool
}

func NewAgentList(loader func(int) tea.Cmd, page, perpage int) AgentListModel {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false // Single-line display only
	l := list.New(nil, d, 0, 0)
	l.Title = "Buildkite Agents"
	l.SetShowStatusBar(false) // Hide the grey count above table

	v := viewport.New(0, 0)
	v.SetContent("")

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
			key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "web")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		}
	}

	l.AdditionalFullHelpKeys = l.AdditionalShortHelpKeys

	return AgentListModel{
		agentList:          l,
		agentViewPort:      v,
		agentDataDisplayed: false,
		agentCurrentPage:   page,
		agentPerPage:       perpage,
		agentLoader:        loader,
	}
}

func (m *AgentListModel) appendAgents() tea.Cmd {
	return m.loadAgents(false)
}

func (m *AgentListModel) refreshAgents() tea.Cmd {
	m.isRefreshing = true
	return m.loadAgents(true)
}

func (m *AgentListModel) loadAgents(isRefresh bool) tea.Cmd {
	// Set agentsLoading
	m.agentsLoading = true
	// Set a status message and start the agentList's spinner
	startSpiner := m.agentList.StartSpinner()

	var statusMessage string
	if isRefresh {
		statusMessage = "Refreshing agents..."
	} else if m.agentCurrentPage == 1 {
		statusMessage = "Loading agents..."
	} else {
		statusMessage = fmt.Sprintf("Loading more agents: page %d of %d", m.agentCurrentPage, m.agentLastPage)
	}

	setStatus := m.agentList.NewStatusMessage(statusMessage)
	// Fetch agents
	loadAgents := m.agentLoader(m.agentCurrentPage)
	return tea.Sequence(tea.Batch(startSpiner, setStatus), loadAgents)
}

func (m *AgentListModel) setComponentSizing(width, height int) {
	m.terminalWidth = width
	m.terminalHeight = height
	h, v := agentListStyle.GetFrameSize()

	// Calculate responsive layout based on screen size
	var listWidth, listHeight, viewportWidth, viewportHeight int

	if m.agentDataDisplayed {
		// For narrow screens, stack vertically (list on top, details on bottom)
		if width < 100 {
			listWidth = width - h
			listHeight = (height - v) * 2 / 3 // List takes 2/3 of height
			viewportWidth = width - h
			viewportHeight = (height - v) / 3 // Details take 1/3 of height
		} else {
			// For wide screens, use side-by-side layout
			if width > 120 {
				listWidth = int(float64(width-h) * 0.6)
				viewportWidth = width - h - listWidth
			} else {
				listWidth = (width - h) / 2
				viewportWidth = (width - h) / 2
			}
			listHeight = height - v
			viewportHeight = height - v
		}
	} else {
		// When details are hidden, list takes full space
		listWidth = width - h
		listHeight = height - v
		viewportWidth = width - h
		viewportHeight = height - v
	}

	// Set component sizes
	m.agentList.SetSize(listWidth, listHeight)
	m.agentViewPort.Height = viewportHeight
	m.agentViewPort.Width = viewportWidth

	// Set styles for rendering
	agentListStyle.Width(listWidth)
	agentListStyle.Height(listHeight)
	viewPortStyle.Width(viewportWidth)
	viewPortStyle.Height(viewportHeight)
}

func (m *AgentListModel) clearAgentViewPort() {
	m.agentViewPort.SetContent("")
	m.agentDataDisplayed = false
}

func (m *AgentListModel) updateTitle() {
	count := len(m.agentList.Items())
	m.agentList.Title = fmt.Sprintf("Buildkite Agents (%d connected)", count)
}

func (m AgentListModel) Init() tea.Cmd {
	return m.appendAgents()
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// When viewport size is reported, start a spinner and show a message to the user indicating agents are loading
		m.setComponentSizing(msg.Width, msg.Height)
		return m, tea.Batch(m.agentList.StartSpinner(), m.agentList.NewStatusMessage("Loading agents"))
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			if !m.agentDataDisplayed {
				if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
					tableContext := AgentDataTable(agent.Agent)
					m.agentViewPort.SetContent(tableContext)
					m.agentDataDisplayed = true
					// Recalculate layout when showing details
					m.setComponentSizing(m.terminalWidth, m.terminalHeight)
				}
			} else {
				m.clearAgentViewPort()
				// Recalculate layout when hiding details
				m.setComponentSizing(m.terminalWidth, m.terminalHeight)
			}

		case "w":
			if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
				if err := browser.OpenURL(agent.WebURL); err != nil {
					return m, m.agentList.NewStatusMessage(fmt.Sprintf("Failed opening agent web url: %s", err.Error()))
				}
			}
		case "r":
			// Refresh agents without clearing the list
			m.agentCurrentPage = 1
			m.agentsLoading = false
			m.clearAgentViewPort()
			// Recalculate layout when hiding details (same as 'v' toggle)
			m.setComponentSizing(m.terminalWidth, m.terminalHeight)
			return m, m.refreshAgents()
		}
	// Custom messages
	case AgentItemsMsg:
		// Handle agent items based on whether we're refreshing or appending
		var allItems []list.Item
		if m.isRefreshing {
			// Replace existing items with new ones on refresh
			allItems = msg.ListItems()
			m.isRefreshing = false
		} else {
			// Append new items for pagination
			allItems = append(m.agentList.Items(), msg.ListItems()...)
		}

		cmds = append(cmds, m.agentList.SetItems(allItems))
		m.updateTitle() // Update title with agent count
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

	// Store the current index before updating the list
	prevIndex := m.agentList.Index()

	var cmd tea.Cmd
	m.agentList, cmd = m.agentList.Update(msg)
	cmds = append(cmds, cmd)

	// Update or clear agent viewport when selection changes
	if m.agentList.Index() != prevIndex {
		if m.agentDataDisplayed {
			// If view is active, update it with the new selection
			if agent, ok := m.agentList.SelectedItem().(AgentListItem); ok {
				tableContext := AgentDataTable(agent.Agent)
				m.agentViewPort.SetContent(tableContext)
			}
		} else {
			m.clearAgentViewPort()
		}
	}

	// Handle pagination when trying to navigate past the last item
	if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "down" {
		wasOnLastItem := prevIndex == len(m.agentList.Items())-1
		stillOnLastItem := m.agentList.Index() == len(m.agentList.Items())-1
		unfilteredState := m.agentList.FilterState() == list.Unfiltered

		// Only trigger pagination if user was already on last item and tried to go down
		if !m.agentsLoading && wasOnLastItem && stillOnLastItem && unfilteredState {
			lastPageReached := m.agentCurrentPage > m.agentLastPage
			if !lastPageReached {
				cmds = append(cmds, m.appendAgents())
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m AgentListModel) View() string {
	// For narrow screens with details shown, stack vertically
	if m.agentDataDisplayed && m.terminalWidth < 100 {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			agentListStyle.Render(m.agentList.View()),
			viewPortStyle.Render(m.agentViewPort.View()),
		)
	}

	// For wide screens or no details, use horizontal layout
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		agentListStyle.Render(m.agentList.View()),
		viewPortStyle.Render(m.agentViewPort.View()),
	)
}
