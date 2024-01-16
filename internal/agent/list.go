package agent

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var agentListStyle = lipgloss.NewStyle().Margin(1, 2)

type agentListModel struct {
	agentList list.Model
	quitting  bool
}

func ObtainAgents(f *factory.Factory, name, version, hostname string) (*agentListModel, error) {
	var alo buildkite.AgentListOptions

	if name != "" || version != "" || hostname != "" {
		alo = buildkite.AgentListOptions{
			Name:     name,
			Version:  version,
			Hostname: hostname,
		}
	}

	// Obtain agent list
	agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, &alo)
	items := make([]list.Item, len(agents))

	if err != nil {
		return nil, err
	}

	for i, agent := range agents {
		items[i] = agentListItem{
			Agent: &agent,
		}
	}

	m := agentListModel{
		agentList: list.New(items, list.NewDefaultDelegate(), 20, 0),
	}

	// Set Title
	m.agentList.Title = "Buildkite Agents"

	return &m, nil
}

func (m agentListModel) Init() tea.Cmd {
	return nil
}

func (m agentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
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

func (m agentListModel) View() string {
	if m.quitting {
		return ""
	} else {
		return agentListStyle.Render(m.agentList.View())
	}
}
