package agent

import (
	"bytes"
	"fmt"
	"time"

	"github.com/buildkite/cli/v3/pkg/style"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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

	v := viewport.New(80, 30)

	return AgentListModel{
		agentList:     l,
		agentViewPort: v,
		agentLoader:   loader,
	}
}

func getAgentTable(agentData AgentListItem) string {
	// Parse metadata and queue name from returned REST API Metadata list
	metadata, queue := ParseMetadata(agentData.Metadata)

	tableOut := &bytes.Buffer{}
	bold := lipgloss.NewStyle().Bold(true)
	agentStateStyle := lipgloss.NewStyle().Bold(true).Foreground(MapStatusToColour(*agentData.ConnectedState))
	queueStyle := lipgloss.NewStyle().Foreground(style.Teal)
	versionStyle := lipgloss.NewStyle().Foreground(style.Grey)

	fmt.Fprint(tableOut, bold.Render(*agentData.Name))

	t := table.New().Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().PaddingRight(2)
	})

	// Construct table row data
	t.Row("ID", *agentData.ID)
	t.Row("State", agentStateStyle.Render(*agentData.ConnectedState))
	t.Row("Queue", queueStyle.Render(queue))
	t.Row("Version", versionStyle.Render(*agentData.Version))
	t.Row("Hostname", *agentData.Hostname)
	// t.Row("PID", *agent.)
	t.Row("User Agent", *agentData.UserAgent)
	t.Row("IP Address", *agentData.IPAddress)
	// t.Row("OS", *agent.)
	t.Row("Connected", agentData.CreatedAt.UTC().Format(time.RFC1123Z))
	// t.Row("Stopped By", *agent.CreatedAt)
	t.Row("Metadata", metadata)

	fmt.Fprint(tableOut, t.Render())
	return tableOut.String()
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
				tableContext := getAgentTable(agent)
				m.agentViewPort.SetContent(tableContext)
			}
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
