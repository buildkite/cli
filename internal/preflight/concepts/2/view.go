// Package concept2 implements the "Oscilloscope" preflight terminal UI.
//
// Design philosophy: treats the terminal like a scientific instrument readout.
// Dense, information-rich, high signal. Compact grid layout with clear visual
// hierarchy — every character earns its place.
package concept2

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// RunState holds the current state of a preflight build.
type RunState struct {
	BuildNumber int
	BuildURL    string
	State       string // scheduled, running, passed, failed, canceled
	Branch      string
	Commit      string
	Jobs        []JobState
	FailedTests []TestFailure
	Elapsed     time.Duration
}

// JobState holds the state of an individual job.
type JobState struct {
	Name       string
	State      string // scheduled, running, passed, failed, canceled
	Duration   time.Duration
	LogExcerpt string
}

// TestFailure holds details of a single test failure.
type TestFailure struct {
	Name     string
	Location string
	Message  string
}

// Render produces the Oscilloscope terminal UI for the given state.
func Render(state *RunState) string {
	w := termWidth()
	noColor := isNoColor()

	var b strings.Builder

	// --- Status bar ---
	statusBar := formatStatusBar(state, noColor)
	b.WriteString("  " + statusBar + "\n")

	// Heavy rule
	rule := strings.Repeat("━", w-4)
	b.WriteString("  " + rule + "\n")

	// --- Jobs section ---
	completed, total := jobCounts(state.Jobs)
	counterText := fmt.Sprintf("%d/%d complete", completed, total)

	sectionHeader := "JOBS"
	headerPad := w - 6 - runewidth.StringWidth(sectionHeader) - runewidth.StringWidth(counterText)
	if headerPad < 1 {
		headerPad = 1
	}
	b.WriteString("\n")
	b.WriteString("  " + sectionHeader + strings.Repeat(" ", headerPad) + counterText + "\n")

	innerW := w - 8 // 2 indent + 2 border + 2 inner padding + 2 margin
	if innerW < 20 {
		innerW = 20
	}
	boxW := w - 4

	b.WriteString("  " + boxTop(boxW) + "\n")
	for _, job := range state.Jobs {
		line := formatJobLine(job, innerW, noColor)
		b.WriteString("  │ " + rpad(line, boxW-4) + " │\n")
	}
	b.WriteString("  " + boxBottom(boxW) + "\n")

	// --- Signals section (only if there are failures) ---
	if len(state.FailedTests) > 0 {
		sigHeader := fmt.Sprintf("SIGNALS  %d failure", len(state.FailedTests))
		if len(state.FailedTests) != 1 {
			sigHeader += "s"
		}
		sigHeader += " detected"

		b.WriteString("\n")
		b.WriteString("  " + sigHeader + "\n")
		b.WriteString("  " + boxTop(boxW) + "\n")
		b.WriteString("  │" + strings.Repeat(" ", boxW-2) + "│\n")

		for i, tf := range state.FailedTests {
			nameStr := tf.Name
			locStr := tf.Location
			msgStr := tf.Message

			if !noColor {
				nameStr = bold(nameStr)
				locStr = dim(locStr)
			}

			b.WriteString("  │  " + rpad(nameStr, boxW-6) + "  │\n")
			b.WriteString("  │  " + rpad(locStr, boxW-6) + "  │\n")
			b.WriteString("  │  " + rpad(msgStr, boxW-6) + "  │\n")

			if i < len(state.FailedTests)-1 {
				b.WriteString("  │" + strings.Repeat(" ", boxW-2) + "│\n")
			}
		}

		b.WriteString("  │" + strings.Repeat(" ", boxW-2) + "│\n")
		b.WriteString("  " + boxBottom(boxW) + "\n")
	}

	// --- Verdict ---
	verdict := state.State
	if verdict == "running" || verdict == "scheduled" {
		verdict = state.State
	}
	verdictLine := "▸ " + verdict
	if !noColor {
		switch state.State {
		case "passed":
			verdictLine = green(verdictLine)
		case "failed":
			verdictLine = red(verdictLine)
		case "canceled":
			verdictLine = yellow(verdictLine)
		case "running":
			verdictLine = cyan(verdictLine)
		default:
			verdictLine = dim(verdictLine)
		}
	}
	b.WriteString("\n")
	b.WriteString("  " + verdictLine + "\n")

	return b.String()
}

// Demo returns a rendered sample for previewing the Oscilloscope concept.
func Demo() string {
	state := &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/acme/preflight/builds/4821",
		State:       "failed",
		Branch:      "main",
		Commit:      "a1b2c3d",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 4*time.Second + 200*time.Millisecond},
			{Name: "Unit Tests", State: "passed", Duration: 12*time.Second + 100*time.Millisecond},
			{Name: "Integration Tests", State: "running", Duration: 1*time.Minute + 2*time.Second},
			{Name: "E2E Tests", State: "scheduled"},
			{Name: "Deploy Preview", State: "scheduled"},
		},
		FailedTests: []TestFailure{
			{
				Name:     "TestUserAuth/login_with_expired_token",
				Location: "pkg/auth/auth_test.go:142",
				Message:  "expected status 401, got 200",
			},
			{
				Name:     "TestAPIRateLimit/concurrent_requests",
				Location: "pkg/api/rate_test.go:89",
				Message:  "context deadline exceeded",
			},
		},
	}
	return Render(state)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func formatStatusBar(s *RunState, noColor bool) string {
	sep := " ─── "

	commit := s.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}

	parts := []string{
		fmt.Sprintf("preflight #%d", s.BuildNumber),
		fmt.Sprintf("%s @ %s", s.Branch, commit),
		s.State,
		formatDuration(s.Elapsed),
	}
	bar := strings.Join(parts, sep)

	if noColor {
		return bar
	}

	// Colorize the state portion
	coloredState := s.State
	switch s.State {
	case "passed":
		coloredState = green(s.State)
	case "failed":
		coloredState = red(s.State)
	case "canceled":
		coloredState = yellow(s.State)
	case "running":
		coloredState = cyan(s.State)
	}
	parts[2] = coloredState
	return strings.Join(parts, sep)
}

func formatJobLine(job JobState, innerW int, noColor bool) string {
	var icon string
	switch job.State {
	case "passed":
		icon = "✓"
		if !noColor {
			icon = green(icon)
		}
	case "failed":
		icon = "✗"
		if !noColor {
			icon = red(icon)
		}
	case "running":
		icon = "●"
		if !noColor {
			icon = cyan(icon)
		}
	case "canceled":
		icon = "○"
		if !noColor {
			icon = yellow(icon)
		}
	default: // scheduled / waiting
		icon = "·"
	}

	name := job.Name

	// Scheduled/waiting jobs: icon + name, nothing else
	if job.State == "scheduled" {
		return icon + " " + name
	}

	dur := formatDuration(job.Duration)
	suffix := ""
	if job.State == "running" {
		suffix = "  ..."
	}
	right := dur + suffix

	// Build dot leaders between name and duration
	nameW := runewidth.StringWidth(name)
	rightW := runewidth.StringWidth(right)
	iconW := runewidth.StringWidth(icon)

	// Layout: icon + " " + name + " " + dots + " " + right
	dotsSpace := innerW - iconW - 1 - nameW - 1 - 1 - rightW
	if dotsSpace < 3 {
		dotsSpace = 3
	}
	dots := strings.Repeat("·", dotsSpace)

	if !noColor {
		dots = dim(dots)
	}

	return icon + " " + name + " " + dots + " " + right
}

func jobCounts(jobs []JobState) (completed, total int) {
	total = len(jobs)
	for _, j := range jobs {
		if j.State == "passed" || j.State == "failed" {
			completed++
		}
	}
	return
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	minutes := int(d.Minutes())
	seconds := d.Seconds() - float64(minutes*60)

	if minutes > 0 {
		// Show whole seconds when there are minutes
		return fmt.Sprintf("%dm %02ds", minutes, int(seconds))
	}
	// Sub-minute: show one decimal
	return fmt.Sprintf("%.1fs", seconds)
}

// ---------------------------------------------------------------------------
// Box drawing
// ---------------------------------------------------------------------------

func boxTop(w int) string {
	if w < 2 {
		w = 2
	}
	return "┌" + strings.Repeat("─", w-2) + "┐"
}

func boxBottom(w int) string {
	if w < 2 {
		w = 2
	}
	return "└" + strings.Repeat("─", w-2) + "┘"
}

// ---------------------------------------------------------------------------
// Terminal utilities
// ---------------------------------------------------------------------------

func termWidth() int {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		w, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && w > 0 {
			return w
		}
	}
	return 80
}

func isNoColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// rpad right-pads s to width w, accounting for wide characters and ANSI escapes.
func rpad(s string, w int) string {
	visible := visibleWidth(s)
	if visible >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visible)
}

// visibleWidth returns the display width of s, stripping ANSI escape sequences.
func visibleWidth(s string) int {
	stripped := stripAnsi(s)
	return runewidth.StringWidth(stripped)
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until we find the terminating letter
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++ // skip the terminating letter
			}
			i = j
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// ANSI helpers
// ---------------------------------------------------------------------------

func bold(s string) string   { return "\x1b[1m" + s + "\x1b[0m" }
func dim(s string) string    { return "\x1b[2m" + s + "\x1b[0m" }
func red(s string) string    { return "\x1b[31m" + s + "\x1b[0m" }
func green(s string) string  { return "\x1b[32m" + s + "\x1b[0m" }
func yellow(s string) string { return "\x1b[33m" + s + "\x1b[0m" }
func cyan(s string) string   { return "\x1b[36m" + s + "\x1b[0m" }
