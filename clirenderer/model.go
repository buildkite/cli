package clirenderer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error

type Model struct {
	spinner  spinner.Model
	quitting bool
	err      error
	reason   string
}

func Create() Model {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return Model{
		spinner: s,
		reason:  loadReason(),
	}
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m Model) View() string {
	if m.err != nil {
		return m.err.Error()
	}
	str := fmt.Sprintf("%s %s...\n", m.spinner.View(), m.reason)

	if m.quitting {
		return str + "\n"
	}
	return str
}

func (m Model) Quit() {
	m.quitting = true
	return
}

func loadReason() string {
	//create the reasons slice and append reasons to it
	reasons := make([]string, 0)
	reasons = append(reasons,
		"Counting bobcats",
		"Buying helicopters",
		"Spending keithbucks",
		"Chasing cars",
		"Calling JJ",
	)
	rand.Seed(time.Now().Unix())
	n := rand.Int() % len(reasons)
	return reasons[n]

}
