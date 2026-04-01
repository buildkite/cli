package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	mutedStyle   = lipgloss.NewStyle().Faint(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	failureStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

type buildCreatedMsg struct {
	buildNumber int
	pipeline    string
	url         string
}

type setSnapshotMsg []string
type appendFailedLineMsg string
type setRunningLinesMsg []string
type setSummaryMsg string
type setResultMsg string

type model struct {
	width        int
	height       int
	spinner      spinner.Model
	snapshot     []string
	buildLines   []string
	buildNumber  int
	failedLines  []string
	runningLines []string
	summaryLine  string
	resultLine   string
	watching     bool
}

func newModel() model {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

	return model{
		spinner:     s,
		summaryLine: "Waiting for build updates...",
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if !m.watching {
			break
		}

		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case setSnapshotMsg:
		m.snapshot = append([]string(nil), msg...)

	case buildCreatedMsg:
		m.buildNumber = msg.buildNumber
		m.buildLines = []string{
			fmt.Sprintf("Pipeline: %s", msg.pipeline),
			fmt.Sprintf("Build:    %s", msg.url),
		}
		m.watching = true
		cmds = append(cmds, m.spinner.Tick)

	case appendFailedLineMsg:
		m.failedLines = append(m.failedLines, string(msg))

	case setRunningLinesMsg:
		m.runningLines = append([]string(nil), msg...)

	case setSummaryMsg:
		m.summaryLine = string(msg)

	case setResultMsg:
		m.resultLine = string(msg)
		m.watching = false
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	width := m.contentWidth()

	sections := []string{
		titleStyle.Render("Bubble Tea Preflight Prototype"),
		mutedStyle.Render(wrapText(
			"Normal screen mode. External messages update independent sections. Resize the terminal to watch wrapped lines reflow. Press q to quit.",
			width,
		)),
		"",
		renderSection("Snapshot", width, m.snapshot),
	}

	if len(m.buildLines) > 0 {
		sections = append(sections, "", renderSection("Build", width, m.buildLines))
	}

	sections = append(sections,
		"",
		renderSection("Status", width, []string{m.statusLine()}),
		"",
		renderSection("Failed Jobs", width, m.failedLines),
		"",
		renderSection("Running Jobs", width, m.runningLines),
		"",
		renderSection("Summary", width, []string{m.summaryLine}),
	)

	if m.resultLine != "" {
		resultStyle := failureStyle
		if strings.Contains(m.resultLine, "passed") {
			resultStyle = successStyle
		}
		sections = append(sections, "", renderSection("Result", width, []string{resultStyle.Render(m.resultLine)}))
	}

	return strings.TrimRight(lipgloss.JoinVertical(lipgloss.Left, sections...), "\n")
}

func (m model) contentWidth() int {
	width := m.width - 2
	if width < 24 {
		return 24
	}
	return width
}

func (m model) statusLine() string {
	statusLine := "Waiting to start watching..."
	if m.watching {
		return fmt.Sprintf("%s Watching build #%d...", m.spinner.View(), m.buildNumber)
	}
	if m.resultLine != "" {
		statusLine = fmt.Sprintf("Watching build #%d complete", m.buildNumber)
	}
	return statusLine
}

func renderSection(title string, width int, lines []string) string {
	parts := []string{headingStyle.Render(title)}
	if len(lines) == 0 {
		parts = append(parts, mutedStyle.Render("  (empty)"))
		return strings.Join(parts, "\n")
	}

	for _, line := range lines {
		parts = append(parts, wrapText(line, width))
	}

	return strings.Join(parts, "\n")
}

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	return wordwrap.String(s, width)
}

func main() {
	p := tea.NewProgram(newModel())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runDemo(ctx, p)

	if _, err := p.Run(); err != nil {
		fmt.Printf("bubbletea demo failed: %v\n", err)
	}
}

func runDemo(ctx context.Context, p *tea.Program) {
	steps := []struct {
		delay time.Duration
		msg   tea.Msg
	}{
		{
			delay: 150 * time.Millisecond,
			msg: setSnapshotMsg{
				"Commit: 24b5e5fd9a",
				"Ref:    refs/heads/bk/preflight/019d2d62-a852-7323-b105-b654e05cc727",
				"Files:  4 changed",
				"  ~ cmd/preflight/render.go",
				"  ~ cmd/preflight/preflight.go",
				"  + internal/preflight/concepts/main.go",
				"  + internal/preflight/concepts/README.md",
			},
		},
		{
			delay: 500 * time.Millisecond,
			msg: buildCreatedMsg{
				buildNumber: 183663,
				pipeline:    "buildkite/cli",
				url:         "https://buildkite.com/buildkite/cli/builds/183663",
			},
		},
		{
			delay: 250 * time.Millisecond,
			msg: setRunningLinesMsg{
				"  ● Lint running for 1m42s",
				"  ● Unit tests running for 1m38s",
				"  ● Integration tests running for 44s",
			},
		},
		{
			delay: 100 * time.Millisecond,
			msg:   setSummaryMsg("  ... 3 running"),
		},
		{
			delay: 900 * time.Millisecond,
			msg:   appendFailedLineMsg("  ✗ ECR Vulnerabilities Scan failed — bk job log -b 183663 -p buildkite/cli failed-ecr-vulnerabilities-scan"),
		},
		{
			delay: 75 * time.Millisecond,
			msg: setRunningLinesMsg{
				"  ● Unit tests running for 2m11s",
				"  ● Integration tests running for 1m17s",
			},
		},
		{
			delay: 75 * time.Millisecond,
			msg:   setSummaryMsg("  ... 1 failed, 2 running, 6 passed"),
		},
		{
			delay: 900 * time.Millisecond,
			msg:   appendFailedLineMsg("  ✗ Integration tests failed with exit 14 — bk job log -b 183663 -p buildkite/cli failed-integration-tests"),
		},
		{
			delay: 75 * time.Millisecond,
			msg: setRunningLinesMsg{
				"  ● Unit tests running for 2m56s",
			},
		},
		{
			delay: 75 * time.Millisecond,
			msg:   setSummaryMsg("  ... 2 failed, 1 running, 8 passed"),
		},
		{
			delay: 900 * time.Millisecond,
			msg:   setRunningLinesMsg{},
		},
		{
			delay: 75 * time.Millisecond,
			msg:   setSummaryMsg("  ... 2 failed, 9 passed"),
		},
		{
			delay: 75 * time.Millisecond,
			msg:   setResultMsg("❌ Preflight failed"),
		},
	}

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return
		case <-time.After(step.delay):
			p.Send(step.msg)
		}
	}
}
