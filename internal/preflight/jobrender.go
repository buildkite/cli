package preflight

import (
	"fmt"
	"strings"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// ANSI color/style codes used for job rendering.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner returns an animated spinner character based on wall-clock time,
// cycling through frames at ~80ms each regardless of poll rate.
func Spinner(_ int) string {
	idx := int(time.Now().UnixMilli()/80) % len(spinnerFrames)
	return ansiCyan + spinnerFrames[idx] + ansiReset
}

// JobDisplayName returns a human-readable name for a job.
func JobDisplayName(j buildkite.Job) string {
	if j.Name != "" {
		return j.Name
	}
	if j.Label != "" {
		return j.Label
	}
	if j.Type == "waiter" {
		return "— wait —"
	}
	if j.Type == "manual" {
		return "▶ block"
	}
	return j.Type + " step"
}

// IsJobTerminal returns true if the job has reached a final state.
func IsJobTerminal(j buildkite.Job) bool {
	switch j.State {
	case "passed", "failed", "canceled", "timed_out", "skipped",
		"broken", "not_run", "finished":
		return true
	}
	return false
}

// IsJobActive returns true if the job is currently executing.
func IsJobActive(j buildkite.Job) bool {
	switch j.State {
	case "running", "canceling":
		return true
	}
	return false
}

// JobDuration returns the elapsed duration for a job.
func JobDuration(j buildkite.Job) time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if j.FinishedAt != nil {
		end = j.FinishedAt.Time
	}
	return end.Sub(j.StartedAt.Time).Truncate(time.Second)
}

func stateIcon(j buildkite.Job) string {
	if j.SoftFailed {
		return ansiYellow + "⚠" + ansiReset
	}
	switch j.State {
	case "passed":
		return ansiGreen + "✓" + ansiReset
	case "failed", "broken":
		return ansiRed + "✗" + ansiReset
	case "canceled":
		return ansiGray + "⊘" + ansiReset
	case "timed_out":
		return ansiRed + "⏱" + ansiReset
	case "running":
		return ansiCyan + "●" + ansiReset
	case "scheduled", "assigned", "accepted":
		return ansiYellow + "○" + ansiReset
	case "waiting", "blocked", "limiting":
		return ansiGray + "◌" + ansiReset
	case "skipped", "not_run":
		return ansiGray + "–" + ansiReset
	case "canceling":
		return ansiYellow + "⊘" + ansiReset
	default:
		return ansiGray + "?" + ansiReset
	}
}

func stateLabel(j buildkite.Job) string {
	label := j.State
	if j.SoftFailed {
		label = "soft failed"
	}
	switch j.State {
	case "passed":
		return ansiGreen + label + ansiReset
	case "failed", "broken", "timed_out":
		return ansiRed + label + ansiReset
	case "running", "canceling":
		return ansiCyan + label + ansiReset
	case "scheduled", "assigned", "accepted":
		return ansiYellow + label + ansiReset
	default:
		return ansiGray + label + ansiReset
	}
}

// FormatTerminalJob renders a completed job line for permanent scrollback.
// Failed jobs include the job ID for easy lookup.
func FormatTerminalJob(j buildkite.Job) string {
	dur := ""
	if d := JobDuration(j); d > 0 {
		dur = ansiDim + " (" + d.String() + ")" + ansiReset
	}
	exit := ""
	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		exit = ansiRed + fmt.Sprintf(" exit %d", *j.ExitStatus) + ansiReset
	}
	id := ""
	if j.State == "failed" || j.State == "timed_out" || j.SoftFailed {
		id = ansiDim + " " + j.ID + ansiReset
	}
	return fmt.Sprintf("  %s %-50s %s%s%s%s",
		stateIcon(j), JobDisplayName(j), stateLabel(j), dur, exit, id)
}

// FormatLiveJob renders an in-progress job line for the live region.
func FormatLiveJob(j buildkite.Job) string {
	dur := ""
	if d := JobDuration(j); d > 0 {
		dur = ansiDim + " " + d.String() + ansiReset
	}
	return fmt.Sprintf("  %s %-50s %s%s",
		stateIcon(j), JobDisplayName(j), stateLabel(j), dur)
}

// IsParallelJob returns true if the job belongs to a parallelism group.
func IsParallelJob(j buildkite.Job) bool {
	return j.ParallelGroupTotal != nil && *j.ParallelGroupTotal > 1
}

// ParallelGroup tracks the state counts for a set of jobs with the same name.
type ParallelGroup struct {
	Name       string
	Total      int
	Running    int
	Passed     int
	Failed     int
	Waiting    int
	Order      int // insertion order for stable rendering
	FailedJobs []buildkite.Job
}

// FormatParallelGroupLive renders summary lines for an active parallel group.
// Failed jobs are shown indented under the group header.
func FormatParallelGroupLive(g *ParallelGroup) []string {
	var parts []string
	if g.Running > 0 {
		parts = append(parts, fmt.Sprintf("%s%d running%s", ansiCyan, g.Running, ansiReset))
	}
	if g.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d passed%s", ansiGreen, g.Passed, ansiReset))
	}
	if g.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d failed%s", ansiRed, g.Failed, ansiReset))
	}
	if g.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%s%d waiting%s", ansiGray, g.Waiting, ansiReset))
	}

	icon := ansiCyan + "●" + ansiReset
	if g.Running == 0 && g.Failed > 0 {
		icon = ansiRed + "✗" + ansiReset
	} else if g.Running == 0 {
		icon = ansiYellow + "○" + ansiReset
	}

	summary := strings.Join(parts, ", ")
	lines := []string{
		fmt.Sprintf("  %s %-50s %s %s(%d)%s",
			icon, g.Name, summary, ansiDim, g.Total, ansiReset),
	}
	for _, j := range g.FailedJobs {
		lines = append(lines, formatFailedGroupJob(j))
	}
	return lines
}

// FormatParallelGroupTerminal renders a completed parallel group for scrollback.
// Failed jobs are shown indented under the group header.
func FormatParallelGroupTerminal(g *ParallelGroup) []string {
	icon := ansiGreen + "✓" + ansiReset
	label := ansiGreen + "passed" + ansiReset
	if g.Failed > 0 {
		icon = ansiRed + "✗" + ansiReset
		label = fmt.Sprintf("%s%d/%d failed%s", ansiRed, g.Failed, g.Total, ansiReset)
	}
	lines := []string{
		fmt.Sprintf("  %s %-50s %s", icon, g.Name, label),
	}
	for _, j := range g.FailedJobs {
		lines = append(lines, formatFailedGroupJob(j))
	}
	return lines
}

func formatFailedGroupJob(j buildkite.Job) string {
	dur := ""
	if d := JobDuration(j); d > 0 {
		dur = ansiDim + " (" + d.String() + ")" + ansiReset
	}
	exit := ""
	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		exit = ansiRed + fmt.Sprintf(" exit %d", *j.ExitStatus) + ansiReset
	}
	return fmt.Sprintf("      %s %s%s%s %s%s%s",
		ansiRed+"✗"+ansiReset, stateLabel(j), dur, exit, ansiDim, j.ID, ansiReset)
}
