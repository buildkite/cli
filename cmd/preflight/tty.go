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
				line := timestampPrefix(msg.Time) + presenter.ColoredLine(*msg.Job)
				return m, tea.Printf("%s", m.hardwrapLine(line))
			}

		case EventBuildSummary:
			// Print the summary via Printf (which scrolls it above the
			// view) instead of rendering it through View(). Inline-image
			// escape sequences from emoji.Render confuse Bubbletea's
			// cursor tracking, causing lines to vanish on re-render.
			m.summary = &msg
			return m, tea.Sequence(
				tea.Printf("%s", buildSummaryView(msg, m.width)),
				tea.Quit,
			)

		case EventTestFailure:
			if len(msg.TestFailures) > 0 {
				presenter := testPresenter{}
				var cmds []tea.Cmd
				for _, t := range msg.TestFailures {
					line := formatTimestampedBlock(presenter.ColoredLine(t), msg.Time)
					cmds = append(cmds, tea.Printf("%s", m.hardwrapLine(line)))
				}
				return m, tea.Batch(cmds...)
			}
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

	statusLine := fmt.Sprintf("  %s %s", m.spinner.View(), ttyStatusStyle.Render(m.statusText()))

	if m.latest.Jobs == nil {
		return separator + "\n" + statusLine
	}

	parts := make([]string, 0, 6)
	appendPart := func(count int, text string) {
		if count > 0 {
			parts = append(parts, text)
		}
	}
	appendPart(m.latest.Jobs.Passed, fmt.Sprintf("%d passed", m.latest.Jobs.Passed))
	appendPart(m.latest.Jobs.Failed, ttyFailureStyle.Render(fmt.Sprintf("%d failed", m.latest.Jobs.Failed)))
	appendPart(m.latest.Jobs.SoftFailed, ttySoftFailureStyle.Render(fmt.Sprintf("%d soft failed", m.latest.Jobs.SoftFailed)))
	appendPart(m.latest.Jobs.Running, fmt.Sprintf("%d running", m.latest.Jobs.Running))
	appendPart(m.latest.Jobs.Scheduled, fmt.Sprintf("%d scheduled", m.latest.Jobs.Scheduled))
	appendPart(m.latest.Jobs.Waiting, fmt.Sprintf("%d waiting", m.latest.Jobs.Waiting))

	if len(parts) == 0 {
		return separator + "\n" + statusLine
	}

	summaryLine := fmt.Sprintf("  %s", ttyDimStyle.Render(strings.Join(parts, ", ")))
	return separator + "\n" + statusLine + "\n" + summaryLine
}

func (m ttyModel) View() string {
	if m.summary != nil {
		// Summary was already printed via tea.Printf; return empty
		// so Bubbletea clears the spinner area on exit.
		return ""
	}
	return m.hardwrapLine(m.render())
}

// buildSummaryView renders the final build summary for TTY output.
func buildSummaryView(e Event, width int) string {
	style := ttyFailureStyle
	if e.BuildState == "passed" {
		style = ttyStatusStyle
	}

	separator := ttyBorderStyle.Render("─────────────────────────────────────────────")
	out := separator + "\n" + style.Render(summaryHeader(e))
	if label := summaryBuildLabel(e); label != "" && e.BuildURL != "" {
		out += "\n  " + ttyDimStyle.Render(label)
		buildURL := "  " + ttyDimStyle.Render(e.BuildURL)
		if width > 0 {
			buildURL = ansi.Hardwrap(buildURL, width, false)
		}
		out += "\n" + buildURL
	}

	presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
	for _, j := range e.PassedJobs {
		out += "\n  " + presenter.ColoredPassedLine(j, ttyDimStyle)
	}
	out += buildSummaryDetails(e, true, width)

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
