package agent

import (
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type agentListItem struct {
	title, metadata string
}

func (ali agentListItem) Title() string       { return ali.title }
func (ali agentListItem) Description() string { return ali.metadata }
func (ali agentListItem) FilterValue() string { return ali.title }

type AgentListModel struct {
	agentList list.Model
	agents    []buildkite.Agent
	quitting  bool
}

func ObtainAgents(f *factory.Factory, args ...string) (*AgentListModel, error) {
	var alo buildkite.AgentListOptions
	var items []list.Item

	if args[0] != "" || args[1] != "" || args[2] != "" {
		alo = buildkite.AgentListOptions{
			Name:     args[0],
			Version:  args[1],
			Hostname: args[2],
		}
	}

	// Obtain agent list
	agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &alo)

	if err != nil {
		return nil, err
	}

	for _, agent := range agents {
		items = append(items, agentListItem{
			title:    *agent.Name,
			metadata: fmt.Sprintf("%s | v%s | %s", *agent.ID, *agent.Version, *agent.ConnectedState),
		})
	}

	m := AgentListModel{
		agentList: list.New(items, list.NewDefaultDelegate(), 20, 0),
		agents:    agents,
	}

	// Set Title
	m.agentList.Title = "Buildkite Agents"

	return &m, nil
}

func (m AgentListModel) Init() tea.Cmd {
	return nil
}

func (m AgentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := agentListStyle.GetFrameSize()
		m.agentList.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.agentList, cmd = m.agentList.Update(msg)
	return m, cmd
}

func (m AgentListModel) View() string {
	if m.quitting {
		return ""
	} else {
		return agentListStyle.Render(m.agentList.View())
	}
}
