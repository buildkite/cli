package concept3

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

// JobState represents the state of a single job in the build.
type JobState struct {
	Name       string
	State      string // scheduled, running, passed, failed, canceled
	Duration   time.Duration
	LogExcerpt string
}

// TestFailure represents a single failed test.
type TestFailure struct {
	Name     string
	Location string
	Message  string
}

type style struct {
	dim       string
	bold      string
	boldWhite string
	reset     string
}

func newStyle() style {
	if os.Getenv("NO_COLOR") != "" || !isTerminal() {
		return style{}
	}
	return style{
		dim:       "\033[2m",
		bold:      "\033[1m",
		boldWhite: "\033[1;97m",
		reset:     "\033[0m",
	}
}

func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func truncateCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// Render produces the terminal output for the given build state.
func Render(state *RunState) string {
	s := newStyle()
	w := termWidth()
	var b strings.Builder

	// Line 1: build info — all dim
	header := fmt.Sprintf("  preflight #%d  %s  %s  %s",
		state.BuildNumber, state.Branch, truncateCommit(state.Commit), formatDuration(state.Elapsed))
	b.WriteString(s.dim)
	b.WriteString(header)
	b.WriteString(s.reset)
	b.WriteByte('\n')

	// Line 2: job summary — inline
	b.WriteString("  ")
	hasFailed := false
	for i, job := range state.Jobs {
		if i > 0 {
			b.WriteString("  ")
		}
		icon, iconStyle := jobIcon(job.State, s)
		b.WriteString(iconStyle)
		b.WriteString(icon)
		b.WriteString(s.reset)
		b.WriteByte(' ')
		if job.State == "failed" {
			hasFailed = true
			b.WriteString(s.boldWhite)
			b.WriteString(job.Name)
			b.WriteString(s.reset)
		} else {
			b.WriteString(s.dim)
			b.WriteString(job.Name)
			b.WriteString(s.reset)
		}
	}
	b.WriteByte('\n')

	if !hasFailed && state.State != "failed" {
		// All passing or still running — just the status line
		b.WriteString("  ")
		b.WriteString(s.dim)
		b.WriteString(stateLabel(state.State))
		b.WriteString(s.reset)
		b.WriteByte('\n')
		return b.String()
	}

	// Blank line before failures
	b.WriteByte('\n')

	// Expand failed jobs
	for _, job := range state.Jobs {
		if job.State != "failed" {
			continue
		}

		// Failed job header: bold name, then right-aligned "failed  duration"
		name := "  " + s.boldWhite + "✗ " + job.Name + s.reset
		suffix := s.dim + "failed  " + formatDuration(job.Duration) + s.reset
		nameWidth := runewidth.StringWidth(stripAnsi(name))
		suffixWidth := runewidth.StringWidth(stripAnsi(suffix))
		gap := w - nameWidth - suffixWidth
		if gap < 2 {
			gap = 2
		}
		b.WriteString(name)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(suffix)
		b.WriteByte('\n')
		b.WriteByte('\n')

		// Test failures for this job
		tests := testsForJob(state.FailedTests, job.Name)
		if len(tests) == 0 && job.LogExcerpt != "" {
			// No structured test failures — show log excerpt
			for _, line := range strings.Split(strings.TrimSpace(job.LogExcerpt), "\n") {
				b.WriteString("    ")
				b.WriteString(s.dim)
				b.WriteString(line)
				b.WriteString(s.reset)
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
		}
		for i, t := range tests {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("    ")
			b.WriteString(s.boldWhite)
			b.WriteString(t.Name)
			b.WriteString(s.reset)
			b.WriteByte('\n')
			if t.Location != "" {
				b.WriteString("    ")
				b.WriteString(s.dim)
				b.WriteString(t.Location)
				b.WriteString(s.reset)
				b.WriteByte('\n')
			}
			if t.Message != "" {
				b.WriteString("    ")
				b.WriteString(t.Message)
				b.WriteByte('\n')
			}
		}
		if len(tests) > 0 {
			b.WriteByte('\n')
		}
	}

	// Final status line
	testCount := countFailedTests(state)
	b.WriteString("  ")
	b.WriteString(s.boldWhite)
	if testCount > 0 {
		b.WriteString(fmt.Sprintf("failed  %d test", testCount))
		if testCount != 1 {
			b.WriteByte('s')
		}
	} else {
		b.WriteString("failed")
	}
	b.WriteString(s.reset)
	b.WriteByte('\n')

	return b.String()
}

func jobIcon(state string, s style) (string, string) {
	switch state {
	case "passed":
		return "✓", s.dim
	case "failed":
		return "✗", s.boldWhite
	case "running":
		return "●", s.dim
	case "canceled":
		return "✗", s.dim
	default:
		return "·", s.dim
	}
}

func stateLabel(state string) string {
	switch state {
	case "passed":
		return "passed"
	case "failed":
		return "failed"
	case "running":
		return "running"
	case "canceled":
		return "canceled"
	case "scheduled":
		return "scheduled"
	default:
		return state
	}
}

// testsForJob returns test failures. Since TestFailure doesn't carry a job
// reference, all failures are shown under any failed job. For single-job
// failures this is exact; for multi-job failures caller should pre-filter.
func testsForJob(tests []TestFailure, _ string) []TestFailure {
	return tests
}

func countFailedTests(state *RunState) int {
	return len(state.FailedTests)
}

func stripAnsi(s string) string {
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

// Demo returns sample output showing the failure state.
func Demo() string {
	state := &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "failed",
		Branch:      "main",
		Commit:      "a1b2c3d4e5f6",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 12 * time.Second},
			{Name: "Unit Tests", State: "passed", Duration: 45 * time.Second},
			{Name: "Integration Tests", State: "failed", Duration: 1*time.Minute + 2*time.Second},
			{Name: "E2E", State: "scheduled"},
			{Name: "Deploy", State: "scheduled"},
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

// DemoPassing returns sample output showing the minimal passing state.
func DemoPassing() string {
	state := &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "passed",
		Branch:      "main",
		Commit:      "a1b2c3d4e5f6",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 12 * time.Second},
			{Name: "Unit Tests", State: "passed", Duration: 45 * time.Second},
			{Name: "Integration", State: "passed", Duration: 1*time.Minute + 2*time.Second},
			{Name: "E2E", State: "passed", Duration: 1*time.Minute + 50*time.Second},
			{Name: "Deploy", State: "passed", Duration: 30 * time.Second},
		},
	}
	return Render(state)
}
