package concept1

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// RunState holds the complete state of a preflight build.
type RunState struct {
	BuildNumber int
	BuildURL    string
	State       string // scheduled, running, passed, failed, canceled
	Branch      string
	Commit      string // short SHA
	Jobs        []JobState
	FailedTests []TestFailure
	Elapsed     time.Duration
}

// JobState holds the state of a single job within the build.
type JobState struct {
	Name       string
	State      string // scheduled, running, passed, failed, canceled
	Duration   time.Duration
	LogExcerpt string
}

// TestFailure holds details of a single failed test.
type TestFailure struct {
	Name     string
	Location string
	Message  string
}

const (
	margin   = "   "
	minWidth = 40
	maxWidth = 80
)

// ansi helpers — all return empty strings when color is disabled.
type style struct {
	noColor bool
}

func (s style) bold(text string) string {
	if s.noColor {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func (s style) dim(text string) string {
	if s.noColor {
		return text
	}
	return "\033[2m" + text + "\033[0m"
}

func (s style) boldIf(cond bool, text string) string {
	if cond {
		return s.bold(text)
	}
	return text
}

func detectWidth() int {
	fd := int(os.Stdout.Fd())
	if isatty.IsTerminal(uintptr(fd)) || isatty.IsCygwinTerminal(uintptr(fd)) {
		w, _, err := term.GetSize(fd)
		if err == nil && w > 0 {
			return w
		}
	}
	return maxWidth
}

func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

func contentWidth() int {
	w := detectWidth()
	if w > maxWidth {
		w = maxWidth
	}
	if w < minWidth {
		w = minWidth
	}
	return w
}

// Render produces the full terminal output for the given RunState.
func Render(state *RunState) string {
	w := contentWidth()
	s := style{noColor: noColor()}
	var b strings.Builder

	writeRule := func() {
		// ╶──...──╴  with 2-space left margin
		inner := w - 2 - 2 // 2 for margin before ╶, 2 for ╶ and ╴
		if inner < 2 {
			inner = 2
		}
		b.WriteString("  \u2576")
		b.WriteString(strings.Repeat("\u2500", inner))
		b.WriteString("\u2574")
		b.WriteString("\n")
	}

	writeLine := func(left, right string) {
		leftWidth := runewidth.StringWidth(stripANSI(left))
		rightWidth := runewidth.StringWidth(stripANSI(right))
		avail := w - len(margin) - rightWidth
		pad := 0
		if avail > leftWidth {
			pad = avail - leftWidth
		}
		b.WriteString(margin)
		b.WriteString(left)
		if right != "" {
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(right)
		}
		b.WriteString("\n")
	}

	// Top rule
	b.WriteString("\n")
	writeRule()
	b.WriteString("\n")

	// Header: PREFLIGHT #NNNN                         Xm XXs elapsed
	writeLine(
		s.bold("PREFLIGHT")+"  #"+fmt.Sprintf("%d", state.BuildNumber),
		s.dim(formatDuration(state.Elapsed)+" elapsed"),
	)
	b.WriteString("\n")

	// Metadata
	writeLine(s.dim("Branch")+"    "+state.Branch, "")
	writeLine(s.dim("Commit")+"    "+state.Commit, "")
	writeLine(s.dim("Status")+"    "+stateLabel(s, state.State), "")

	b.WriteString("\n")
	writeRule()
	b.WriteString("\n")

	// OBSERVATIONS
	jobCount := len(state.Jobs)
	writeLine(
		s.bold("OBSERVATIONS"),
		s.dim(fmt.Sprintf("%d jobs", jobCount)),
	)
	b.WriteString("\n")

	for _, job := range state.Jobs {
		icon := jobIcon(s, job.State)
		dur := ""
		switch job.State {
		case "passed", "failed":
			dur = formatDuration(job.Duration)
		case "running":
			dur = formatDuration(job.Duration) + "  ..."
		default:
			dur = "waiting"
		}
		writeLine("    "+icon+"  "+job.Name, s.dim(dur))
	}

	b.WriteString("\n")
	writeRule()

	// FAILURES (only if there are any)
	if len(state.FailedTests) > 0 {
		b.WriteString("\n")
		writeLine(
			s.bold("FAILURES"),
			s.dim(fmt.Sprintf("%d tests", len(state.FailedTests))),
		)
		b.WriteString("\n")

		for i, t := range state.FailedTests {
			writeLine(fmt.Sprintf("   %d. %s", i+1, t.Name), "")
			writeLine("      "+s.dim(t.Location), "")
			writeLine("      "+s.dim(t.Message), "")
			if i < len(state.FailedTests)-1 {
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
		writeRule()
	}

	// RESULT
	b.WriteString("\n")
	resultWord := stateLabel(s, state.State)
	writeLine(s.bold("RESULT")+"  "+resultWord, "")

	b.WriteString("\n")

	return b.String()
}

func jobIcon(s style, state string) string {
	switch state {
	case "passed":
		return s.dim("\u2713") // ✓
	case "failed":
		return s.bold("\u2717") // ✗
	case "running":
		return "\u25cf" // ●
	default:
		return s.dim("\u00b7") // ·
	}
}

func stateLabel(s style, state string) string {
	switch state {
	case "passed":
		return "passed"
	case "failed":
		return s.bold("failed")
	case "running":
		return s.dim("running")
	case "canceled":
		return s.dim("canceled")
	default:
		return s.dim(state)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	m := int(d.Minutes())
	sec := d.Seconds() - float64(m*60)
	if m > 0 {
		return fmt.Sprintf("%dm %02.0fs", m, sec)
	}
	// Show one decimal for durations under a minute
	if sec < 10 {
		return fmt.Sprintf("%.1fs", sec)
	}
	return fmt.Sprintf("%.1fs", sec)
}

// stripANSI removes ANSI escape sequences for width calculation.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Demo creates sample data and renders it for previewing the concept.
func Demo() string {
	state := &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "failed",
		Branch:      "main",
		Commit:      "a1b2c3d4ef",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 4200 * time.Millisecond},
			{Name: "Unit Tests", State: "passed", Duration: 12100 * time.Millisecond},
			{Name: "Integration Tests", State: "failed", Duration: 1*time.Minute + 2*time.Second},
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
