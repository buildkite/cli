package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	ttyDimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ttyStatusStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFBA03")).Bold(true)
	ttyBorderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	ttyFailureStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	ttySoftFailureStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

type ttyModel struct {
	spinner    spinner.Model
	latest     Event
	summary    *Event
	cancelFunc context.CancelFunc
	width      int
}

func newTTYModel() ttyModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#DE8F0C"))
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
			timestamp := ttyDimStyle.Render(msg.Time.Format("15:04:05"))
			prefix := timestamp + " "
			line := prefix + msg.Title
			if msg.Detail != "" {
				detail := indentAllLines(msg.Detail, len("15:04:05 "))
				line += ":\n" + detail
			}
			return m, tea.Printf("%s", m.hardwrapLine(line))

		case EventBuildStatus:
			m.latest = msg
			return m, nil

		case EventJobFailure:
			if msg.Job != nil {
				presenter := jobPresenter{pipeline: msg.Pipeline, buildNumber: msg.BuildNumber}
				line := fmt.Sprintf("%s %s",
					ttyDimStyle.Render(msg.Time.Format("15:04:05")),
					presenter.ColoredLine(*msg.Job),
				)
				return m, tea.Printf("%s", m.hardwrapLine(line))
			}

		case EventBuildSummary:
			// Store the summary on the model so View() renders it as the
			// final frame when the program quits via Close().
			m.summary = &msg
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

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
		link := fmt.Sprintf("\033]8;;%s\033\\build #%d\033]8;;\033\\", m.latest.BuildURL, m.latest.BuildNumber)
		return fmt.Sprintf("Watching %s (%s)", link, m.latest.BuildState)
	default:
		return "Starting..."
	}
}

// hardwrapLine pre-wraps text with explicit newlines at the terminal width so that
// Bubbletea's line counting matches the physical rows the terminal will use.
// This prevents cursor positioning errors that leave View() artifacts in the scrollback.
func (m ttyModel) hardwrapLine(s string) string {
	if m.width <= 0 {
		return s
	}
	return ansi.Hardwrap(s, m.width, false)
}

func (m ttyModel) render() string {
	separator := ttyBorderStyle.Render("─────────────────────────────────────────────")

	if m.summary != nil {
		return buildSummaryView(*m.summary)
	}

	statusLine := fmt.Sprintf("  %s %s", m.spinner.View(), ttyStatusStyle.Render(m.statusText()))

	if m.latest.Jobs == nil {
		return separator + "\n" + statusLine
	}

	var parts []string
	if m.latest.Jobs.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", m.latest.Jobs.Passed))
	}
	if m.latest.Jobs.Failed > 0 {
		parts = append(parts, ttyFailureStyle.Render(fmt.Sprintf("%d failed", m.latest.Jobs.Failed)))
	}
	if m.latest.Jobs.SoftFailed > 0 {
		parts = append(parts, ttySoftFailureStyle.Render(fmt.Sprintf("%d soft failed", m.latest.Jobs.SoftFailed)))
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

func (m ttyModel) View() string {
	return m.hardwrapLine(m.render())
}

// buildSummaryView renders the final build summary as a string for use in View().
func buildSummaryView(e Event) string {
	style := ttyFailureStyle
	if e.BuildState == "passed" {
		style = ttyStatusStyle
	}

	separator := ttyBorderStyle.Render("─────────────────────────────────────────────")
	out := separator + "\n" + style.Render(summaryHeader(e))

	presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
	for _, j := range e.PassedJobs {
		out += "\n  " + presenter.ColoredPassedLine(j, ttyDimStyle)
	}
	for _, j := range e.FailedJobs {
		out += "\n  " + presenter.ColoredLine(j)
	}

	return out
}

type ttyRenderer struct {
	program *tea.Program
	done    chan struct{}
	err     error
}

func newTTYRenderer(cancel context.CancelFunc) *ttyRenderer {
	model := newTTYModel()
	model.cancelFunc = cancel
	p := tea.NewProgram(model)
	r := &ttyRenderer{program: p, done: make(chan struct{})}
	go func() {
		if _, err := p.Run(); err != nil {
			r.err = err
		}
		close(r.done)
	}()
	return r
}

func (r *ttyRenderer) Render(e Event) error {
	r.program.Send(e)
	return nil
}

func (r *ttyRenderer) Close() error {
	r.program.Quit()
	<-r.done
	return r.err
}
