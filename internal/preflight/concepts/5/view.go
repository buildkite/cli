// Package concept5 implements "Aperture" — a camera-aperture-inspired terminal UI
// for Buildkite preflight builds. The UI focuses on what matters and blurs everything
// else: the active job is prominent, completed jobs collapse, and failures fill the frame.
package concept5

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// RunState represents the full state of a preflight build.
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

// --- ANSI helpers ---

var noColor = os.Getenv("NO_COLOR") != ""

func bold(s string) string {
	if noColor {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

func dim(s string) string {
	if noColor {
		return s
	}
	return "\033[2m" + s + "\033[0m"
}

// --- layout helpers ---

func termWidth() int {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// contentWidth returns the usable width inside the 2-space margins.
func contentWidth(tw int) int {
	w := tw - 4 // 2 spaces each side
	if w < 40 {
		w = 40
	}
	return w
}

// pad right-aligns `right` against `left` within `width`.
func pad(left, right string, width int) string {
	leftW := runewidth.StringWidth(stripAnsi(left))
	rightW := runewidth.StringWidth(stripAnsi(right))
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// margin wraps a line with 2-space left margin.
func margin(s string) string {
	return "  " + s
}

// stripAnsi removes ANSI escape sequences for width measurement.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// --- formatting helpers ---

func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		secs := d.Seconds()
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm %02ds", mins, secs)
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func stateIcon(state string) string {
	switch state {
	case "passed":
		return "✓"
	case "failed":
		return "✗"
	case "canceled":
		return "⊘"
	default:
		return "·"
	}
}

func dashedRule(w int) string {
	return dim(strings.Repeat("┈", w))
}

func heavyRule(w int) string {
	return strings.Repeat("━", w)
}

// --- job classification ---

func classifyJobs(jobs []JobState) (passed, running, waiting, failed []JobState) {
	for _, j := range jobs {
		switch j.State {
		case "passed":
			passed = append(passed, j)
		case "running":
			running = append(running, j)
		case "failed":
			failed = append(failed, j)
		case "canceled":
			passed = append(passed, j) // treat canceled as background
		default:
			waiting = append(waiting, j)
		}
	}
	return
}

// --- compact job summaries ---

func compactJob(j JobState) string {
	icon := stateIcon(j.State)
	if j.Duration > 0 {
		return fmt.Sprintf("%s %s %s", icon, j.Name, fmtDuration(j.Duration))
	}
	return fmt.Sprintf("%s %s", icon, j.Name)
}

func compactJobLine(jobs []JobState, dimmed bool) string {
	parts := make([]string, len(jobs))
	for i, j := range jobs {
		parts[i] = compactJob(j)
	}
	line := strings.Join(parts, " · ")
	if dimmed {
		return dim(line)
	}
	return line
}

func waitingJobLine(jobs []JobState) string {
	names := make([]string, len(jobs))
	for i, j := range jobs {
		names[i] = j.Name
	}
	return dim("· " + strings.Join(names, " · "))
}

// --- Render produces the full UI string for a given RunState ---

func Render(state *RunState) string {
	tw := termWidth()
	cw := contentWidth(tw)
	var lines []string

	// blank line top
	lines = append(lines, "")

	// Line 1: header
	left := fmt.Sprintf("preflight #%d on %s", state.BuildNumber, state.Branch)
	right := fmtDuration(state.Elapsed)
	lines = append(lines, margin(pad(bold(left), right, cw)))

	switch state.State {
	case "passed":
		lines = append(lines, renderPassed(state, cw)...)
	case "failed":
		lines = append(lines, renderFailed(state, cw)...)
	case "canceled":
		lines = append(lines, renderCanceled(state, cw)...)
	default:
		lines = append(lines, renderRunning(state, cw)...)
	}

	// blank line bottom
	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

// --- passed view: minimal ---

func renderPassed(state *RunState, cw int) []string {
	var lines []string

	countStr := fmt.Sprintf("%d / %d", len(state.Jobs), len(state.Jobs))
	lines = append(lines, margin(pad(bold("passed"), countStr, cw)))

	// blank line
	lines = append(lines, "")

	// all jobs on one line
	lines = append(lines, margin(compactJobLine(state.Jobs, false)))

	return lines
}

// --- failed view: failures fill the frame ---

func renderFailed(state *RunState, cw int) []string {
	var lines []string

	// verdict line
	testCount := len(state.FailedTests)
	countLabel := "test"
	if testCount != 1 {
		countLabel = "tests"
	}
	countStr := fmt.Sprintf("%d %s", testCount, countLabel)
	lines = append(lines, margin(pad(bold("failed"), countStr, cw)))

	// blank line
	lines = append(lines, "")

	// heavy rule
	lines = append(lines, margin(heavyRule(cw)))

	// find failed jobs
	_, _, _, failedJobs := classifyJobs(state.Jobs)

	// failed jobs with their test failures
	for _, fj := range failedJobs {
		lines = append(lines, "")

		// job name with failure icon
		lines = append(lines, margin(pad(bold(fj.Name), bold("✗"), cw)))

		// test failures for this job
		for _, t := range state.FailedTests {
			lines = append(lines, "")
			lines = append(lines, margin("  "+bold(t.Name)))
			if t.Location != "" {
				lines = append(lines, margin("  "+dim(t.Location)))
			}
			if t.Message != "" {
				lines = append(lines, margin("  "+dim(t.Message)))
			}
		}
	}

	// if no failed jobs found but we have test failures, show them standalone
	if len(failedJobs) == 0 && len(state.FailedTests) > 0 {
		for _, t := range state.FailedTests {
			lines = append(lines, "")
			lines = append(lines, margin("  "+bold(t.Name)))
			if t.Location != "" {
				lines = append(lines, margin("  "+dim(t.Location)))
			}
			if t.Message != "" {
				lines = append(lines, margin("  "+dim(t.Message)))
			}
		}
	}

	// blank line
	lines = append(lines, "")

	// heavy rule
	lines = append(lines, margin(heavyRule(cw)))

	// passing jobs compressed line
	passed, _, _, _ := classifyJobs(state.Jobs)
	if len(passed) > 0 {
		lines = append(lines, "")
		lines = append(lines, margin(compactJobLine(passed, false)))
	}

	return lines
}

// --- canceled view ---

func renderCanceled(state *RunState, cw int) []string {
	var lines []string
	lines = append(lines, margin(pad(dim("canceled"), "", cw)))
	lines = append(lines, "")
	lines = append(lines, margin(compactJobLine(state.Jobs, true)))
	return lines
}

// --- running view: focus on the active job ---

func renderRunning(state *RunState, cw int) []string {
	var lines []string

	passed, running, waiting, failed := classifyJobs(state.Jobs)
	doneCount := len(passed) + len(failed)
	totalCount := len(state.Jobs)

	// completed jobs summary + progress count
	if len(passed) > 0 || len(failed) > 0 {
		done := append(passed, failed...)
		compactLine := compactJobLine(done, true)
		countStr := fmt.Sprintf("%d / %d", doneCount, totalCount)
		lines = append(lines, "")
		lines = append(lines, margin(pad(compactLine, countStr, cw)))
	}

	// dashed rule above active
	if len(running) > 0 {
		lines = append(lines, "")
		lines = append(lines, margin(dashedRule(cw)))
	}

	// focused active job(s)
	for _, rj := range running {
		lines = append(lines, "")
		dur := fmtDuration(rj.Duration)
		lines = append(lines, margin(pad(bold(rj.Name), dur, cw)))
	}

	// dashed rule below active
	if len(running) > 0 {
		lines = append(lines, "")
		lines = append(lines, margin(dashedRule(cw)))
	}

	// waiting jobs
	if len(waiting) > 0 {
		lines = append(lines, "")
		lines = append(lines, margin(waitingJobLine(waiting)))
	}

	return lines
}

// --- Demo functions ---

// Demo returns the failed state demo.
func Demo() string {
	return Render(demoFailedState())
}

// DemoRunning returns the running state demo.
func DemoRunning() string {
	return Render(demoRunningState())
}

// DemoPassed returns the passed state demo.
func DemoPassed() string {
	return Render(demoPassedState())
}

func demoRunningState() *RunState {
	return &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "running",
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
	}
}

func demoPassedState() *RunState {
	return &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "passed",
		Branch:      "main",
		Commit:      "a1b2c3d",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 4*time.Second + 200*time.Millisecond},
			{Name: "Unit Tests", State: "passed", Duration: 12*time.Second + 100*time.Millisecond},
			{Name: "Integration", State: "passed", Duration: 1*time.Minute + 2*time.Second},
			{Name: "E2E", State: "passed", Duration: 45 * time.Second},
			{Name: "Deploy", State: "passed", Duration: 3 * time.Second},
		},
	}
}

func demoFailedState() *RunState {
	return &RunState{
		BuildNumber: 4821,
		BuildURL:    "https://buildkite.com/buildkite/preflight/builds/4821",
		State:       "failed",
		Branch:      "main",
		Commit:      "a1b2c3d",
		Elapsed:     2*time.Minute + 34*time.Second,
		Jobs: []JobState{
			{Name: "Lint", State: "passed", Duration: 4*time.Second + 200*time.Millisecond},
			{Name: "Unit Tests", State: "passed", Duration: 12*time.Second + 100*time.Millisecond},
			{Name: "Integration Tests", State: "failed", Duration: 1*time.Minute + 2*time.Second},
			{Name: "E2E", State: "passed", Duration: 45 * time.Second},
			{Name: "Deploy", State: "passed", Duration: 3 * time.Second},
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
}
