package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	ttyDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ttyStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	ttyBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

type ttyModel struct {
	spinner    spinner.Model
	latest     Event
	cancelFunc context.CancelFunc
}

func newTTYModel() ttyModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return ttyModel{spinner: s}
}

func (m ttyModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m ttyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, nil
		}

	case Event:
		switch msg.Type {
		case EventOperation:
			m.latest = msg
			text := msg.Title
			if msg.Detail != "" {
				text = msg.Detail
			}
			line := fmt.Sprintf("  %s  %s",
				ttyDimStyle.Render(msg.Time.Format("15:04:05")),
				text,
			)
			return m, tea.Printf("%s", line)

		case EventBuildStatus:
			m.latest = msg
			return m, nil

		case EventJobFailure:
			if msg.Job != nil {
				presenter := ttyJobPresenter{pipeline: msg.Pipeline, buildNumber: msg.BuildNumber}
				line := fmt.Sprintf("  %s  %s",
					ttyDimStyle.Render(msg.Time.Format("15:04:05")),
					presenter.Line(*msg.Job),
				)
				return m, tea.Printf("%s", line)
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m ttyModel) statusText() string {
	switch {
	case m.latest.Title != "":
		return m.latest.Title
	case m.latest.BuildState != "":
		return fmt.Sprintf("Watching build #%d (%s)", m.latest.BuildNumber, m.latest.BuildState)
	default:
		return "Starting..."
	}
}

func (m ttyModel) View() string {
	separator := ttyBorderStyle.Render("─────────────────────────────────────────────")

	statusLine := fmt.Sprintf("  %s %s", m.spinner.View(), ttyStatusStyle.Render(m.statusText()))

	if m.latest.Jobs == nil {
		return separator + "\n" + statusLine
	}

	var parts []string
	if m.latest.Jobs.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", m.latest.Jobs.Passed))
	}
	if m.latest.Jobs.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", m.latest.Jobs.Failed))
	}
	if m.latest.Jobs.SoftFailed > 0 {
		parts = append(parts, fmt.Sprintf("%d soft failed", m.latest.Jobs.SoftFailed))
	}
	if m.latest.Jobs.Running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", m.latest.Jobs.Running))
	}
	if m.latest.Jobs.Scheduled > 0 {
		parts = append(parts, fmt.Sprintf("%d scheduled", m.latest.Jobs.Scheduled))
	}
	if m.latest.Jobs.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", m.latest.Jobs.Waiting))
	}

	if len(parts) == 0 {
		return separator + "\n" + statusLine
	}

	summaryLine := fmt.Sprintf("  %s", ttyDimStyle.Render(strings.Join(parts, ", ")))
	return separator + "\n" + statusLine + "\n" + summaryLine
}

type ttyRenderer struct {
	program *tea.Program
	done    chan struct{}
}

func newTTYRenderer(cancel context.CancelFunc) *ttyRenderer {
	model := newTTYModel()
	model.cancelFunc = cancel
	p := tea.NewProgram(model)
	r := &ttyRenderer{program: p, done: make(chan struct{})}
	go func() {
		p.Run()
		close(r.done)
	}()
	return r
}

func (r *ttyRenderer) Render(e Event) error {
	r.program.Send(e)
	return nil
}

func (r *ttyRenderer) Close() {
	r.program.Quit()
	<-r.done
}
