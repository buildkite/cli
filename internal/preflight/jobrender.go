package preflight

import (
	"fmt"
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

// Spinner returns an animated spinner character for the given tick.
func Spinner(tick int) string {
	return ansiCyan + spinnerFrames[tick%len(spinnerFrames)] + ansiReset
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
	case "running", "assigned", "accepted", "canceling":
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
func FormatTerminalJob(j buildkite.Job) string {
	dur := ""
	if d := JobDuration(j); d > 0 {
		dur = ansiDim + " (" + d.String() + ")" + ansiReset
	}
	exit := ""
	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		exit = ansiRed + fmt.Sprintf(" exit %d", *j.ExitStatus) + ansiReset
	}
	return fmt.Sprintf("  %s %-50s %s%s%s",
		stateIcon(j), JobDisplayName(j), stateLabel(j), dur, exit)
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
