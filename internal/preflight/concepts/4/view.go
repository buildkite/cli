package concept4

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// RunState represents the complete state of a preflight build.
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

// JobState represents the state of an individual job.
type JobState struct {
	Name       string
	State      string // scheduled, running, passed, failed, canceled
	Duration   time.Duration
	LogExcerpt string
}

// TestFailure represents a single test failure.
type TestFailure struct {
	Name     string
	Location string
	Message  string
}

// style holds ANSI codes, respecting NO_COLOR.
type style struct {
	bold  string
	dim   string
	red   string
	green string
	reset string
}

func newStyle() style {
	if os.Getenv("NO_COLOR") != "" {
		return style{}
	}
	fd := os.Stdout.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return style{}
	}
	return style{
		bold:  "\033[1m",
		dim:   "\033[2m",
		red:   "\033[31m",
		green: "\033[32m",
		reset: "\033[0m",
	}
}

func termWidth() int {
	fd := int(os.Stdout.Fd())
	w, _, err := term.GetSize(fd)
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// Render produces the Spectrograph terminal view for a RunState.
func Render(state *RunState) string {
	s := newStyle()
	w := termWidth()
	var b strings.Builder

	commitShort := state.Commit
	if len(commitShort) > 7 {
		commitShort = commitShort[:7]
	}

	// Header
	b.WriteString(fmt.Sprintf("  %spreflight #%d%s\n", s.bold, state.BuildNumber, s.reset))
	b.WriteString(fmt.Sprintf("  %s @ %s\n", state.Branch, commitShort))
	b.WriteString("  ")
	b.WriteString(rule(w - 4))
	b.WriteString("\n")

	// Accumulate elapsed time from jobs for timestamp calculation
	var cumulativeOffset time.Duration

	// Gather test failures keyed by job name
	failuresByJob := buildFailureMap(state)

	for i, job := range state.Jobs {
		b.WriteString("\n")

		// Timestamp on the left margin
		ts := formatTimestamp(job.State, cumulativeOffset)
		b.WriteString(fmt.Sprintf("  %s%s%s   %s%s%s\n", s.dim, ts, s.reset, s.bold, job.Name, s.reset))

		// Result line indented under the name
		b.WriteString(fmt.Sprintf("          %s\n", formatResult(job, s)))

		// If this job failed, check for inline test failures
		if job.State == "failed" {
			failures := failuresByJob[job.Name]
			if len(failures) == 0 {
				// Use all unmatched failures as a fallback for a single failed job
				failures = state.FailedTests
			}
			if len(failures) > 0 {
				writeFailureBracket(&b, failures, s, w)
			}
		}

		// Advance cumulative offset for the next job's timestamp
		if job.State == "passed" || job.State == "failed" {
			cumulativeOffset += job.Duration
		}

		// Timeline spine connector (except after last job)
		if i < len(state.Jobs)-1 {
			b.WriteString(fmt.Sprintf("  %s·%s\n", s.dim, s.reset))
		}
	}

	// Footer
	b.WriteString("\n  ")
	b.WriteString(rule(w - 4))
	b.WriteString("\n")

	verdict := formatVerdict(state.State, s)
	b.WriteString(fmt.Sprintf("  %s  %s\n", verdict, formatDuration(state.Elapsed)))

	return b.String()
}

func buildFailureMap(state *RunState) map[string][]TestFailure {
	// If there's exactly one failed job, all failures belong to it
	failedJobs := 0
	var singleFailed string
	for _, j := range state.Jobs {
		if j.State == "failed" {
			failedJobs++
			singleFailed = j.Name
		}
	}
	m := make(map[string][]TestFailure)
	if failedJobs == 1 {
		m[singleFailed] = state.FailedTests
	}
	return m
}

func rule(width int) string {
	if width <= 0 {
		width = 20
	}
	return strings.Repeat("─", width)
}

func formatTimestamp(state string, offset time.Duration) string {
	switch state {
	case "scheduled":
		return "──:──"
	case "running":
		mins := int(offset.Minutes())
		secs := int(offset.Seconds()) % 60
		return fmt.Sprintf("%02d:%02d", mins, secs)
	default:
		mins := int(offset.Minutes())
		secs := int(offset.Seconds()) % 60
		return fmt.Sprintf("%02d:%02d", mins, secs)
	}
}

func formatResult(job JobState, s style) string {
	switch job.State {
	case "passed":
		return fmt.Sprintf("%s✓%s %s", s.green, s.reset, formatDuration(job.Duration))
	case "failed":
		return fmt.Sprintf("%s✗%s %s", s.red, s.reset, formatDuration(job.Duration))
	case "running":
		return "● running..."
	case "canceled":
		return fmt.Sprintf("%s⊘ canceled%s", s.dim, s.reset)
	default: // scheduled / waiting
		return fmt.Sprintf("%s· waiting%s", s.dim, s.reset)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %02ds", mins, secs)
}

func formatVerdict(state string, s style) string {
	switch state {
	case "passed":
		return fmt.Sprintf("%s%spassed%s", s.bold, s.green, s.reset)
	case "failed":
		return fmt.Sprintf("%s%sfailed%s", s.bold, s.red, s.reset)
	case "canceled":
		return fmt.Sprintf("%s%scanceled%s", s.bold, s.dim, s.reset)
	case "running":
		return fmt.Sprintf("%srunning%s", s.bold, s.reset)
	default:
		return fmt.Sprintf("%s%s%s", s.bold, state, s.reset)
	}
}

func writeFailureBracket(b *strings.Builder, failures []TestFailure, s style, maxWidth int) {
	indent := "          "
	bracketIndent := indent

	// Available width for failure text inside the bracket
	contentWidth := maxWidth - runewidth.StringWidth(bracketIndent) - 4 // "│ " prefix + margin
	if contentWidth < 20 {
		contentWidth = 20
	}

	b.WriteString(fmt.Sprintf("  %s·%s       %s╷%s\n", s.dim, s.reset, s.dim, s.reset))

	for i, f := range failures {
		if i > 0 {
			b.WriteString(fmt.Sprintf("  %s·%s       %s│%s\n", s.dim, s.reset, s.dim, s.reset))
		}
		// Test name
		b.WriteString(fmt.Sprintf("  %s·%s       %s│%s %s%s%s\n",
			s.dim, s.reset, s.dim, s.reset, s.bold, truncate(f.Name, contentWidth), s.reset))
		// Location
		if f.Location != "" {
			b.WriteString(fmt.Sprintf("  %s·%s       %s│%s %s%s%s\n",
				s.dim, s.reset, s.dim, s.reset, s.dim, truncate(f.Location, contentWidth), s.reset))
		}
		// Message
		if f.Message != "" {
			b.WriteString(fmt.Sprintf("  %s·%s       %s│%s %s%s%s\n",
				s.dim, s.reset, s.dim, s.reset, s.red, truncate(f.Message, contentWidth), s.reset))
		}
	}

	b.WriteString(fmt.Sprintf("  %s·%s       %s╵%s\n", s.dim, s.reset, s.dim, s.reset))
}

func truncate(text string, maxWidth int) string {
	if runewidth.StringWidth(text) <= maxWidth {
		return text
	}
	result := ""
	w := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if w+rw+1 > maxWidth {
			return result + "…"
		}
		result += string(r)
		w += rw
	}
	return result
}

// Demo returns a rendered Spectrograph view with sample data.
func Demo() string {
	state := &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "failed",
		Branch:      "main",
		Commit:      "a1b2c3d4e5f6",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{
				Name:     "Lint",
				State:    "passed",
				Duration: 4*time.Second + 200*time.Millisecond,
			},
			{
				Name:     "Unit Tests",
				State:    "passed",
				Duration: 12*time.Second + 100*time.Millisecond,
			},
			{
				Name:     "Integration Tests",
				State:    "failed",
				Duration: 1*time.Minute + 2*time.Second,
			},
			{
				Name:  "E2E Tests",
				State: "running",
			},
			{
				Name:  "Deploy Preview",
				State: "scheduled",
			},
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
