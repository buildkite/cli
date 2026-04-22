package preflight

import (
	"fmt"
	"sort"
	"strings"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type testPresenter struct{}

type summarySuiteColumnWidths struct {
	Label   int
	Failed  int
	Passed  int
	Skipped int
}

func (p testPresenter) Line(t buildkite.BuildTest) string {
	return p.line(t, false)
}

func (p testPresenter) ColoredLine(t buildkite.BuildTest) string {
	return p.line(t, true)
}

func (p testPresenter) SummarySuiteLine(summary internalpreflight.SummaryTestRun, widths summarySuiteColumnWidths) string {
	label := padRightToWidth(summarySuiteLabel(summary.SuiteName, summary.SuiteSlug, "unknown"), widths.Label)
	icon := "✓"
	if summary.Failed > 0 {
		icon = "✗"
	}

	return fmt.Sprintf(
		"%s %s  %*d failed  %*d passed  %*d skipped",
		icon,
		label,
		widths.Failed,
		summary.Failed,
		widths.Passed,
		summary.Passed,
		widths.Skipped,
		summary.Skipped,
	)
}

func (p testPresenter) SummaryFailureLine(failure internalpreflight.SummaryTestFailure, width int, indent string) string {
	suite := summarySuiteLabel(failure.SuiteName, failure.SuiteSlug, "")
	parts := make([]string, 0, 2)
	if location := strings.TrimSpace(failure.Location); location != "" {
		parts = append(parts, location)
	}
	if name := strings.TrimSpace(failure.Name); name != "" {
		parts = append(parts, truncateToWidth(name, 80))
	}

	line := "✗"
	if suite != "" {
		line += fmt.Sprintf(" [%s]", suite)
	}
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " — ")
	}

	if indent == "" {
		if width <= 0 {
			return line
		}
		return ansi.Hardwrap(line, width, false)
	}

	if width <= runewidth.StringWidth(indent) {
		return indent + line
	}

	if width <= 0 {
		return indent + line
	}

	wrapped := ansi.Hardwrap(line, width-runewidth.StringWidth(indent), false)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		lines[i] = indent + lines[i]
	}

	return strings.Join(lines, "\n")
}

func (p testPresenter) line(t buildkite.BuildTest, colored bool) string {
	name := t.Name
	if t.Scope != "" {
		name = t.Scope + " " + name
	}
	name = truncateToWidth(name, 80)

	latestExecution := latestTestExecution(t)

	statusIcon := formatTestStatusIcon(latestExecution, colored)
	line := fmt.Sprintf("%s %s", statusIcon, name)

	if !isFailedTestExecution(latestExecution) {
		return line
	}

	detailParts := make([]string, 0, 2)
	if attemptSummary := testAttemptCounts(t); attemptSummary != "" {
		detailParts = append(detailParts, attemptSummary)
	}
	if location := latestExecution.Location; location != "" {
		detailParts = append(detailParts, location)
	} else if t.Location != "" {
		detailParts = append(detailParts, t.Location)
	}
	if len(detailParts) > 0 {
		line += fmt.Sprintf("\n    %s", formatTestDetail(strings.Join(detailParts, " — "), colored))
	}

	if latestExecution.FailureReason != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(latestExecution.FailureReason, colored))
	}

	return line
}

func testAttemptCounts(t buildkite.BuildTest) string {
	attempts := t.ExecutionsCount
	if attempts == 0 {
		return ""
	}

	passed := t.ExecutionsCountByResult.Passed
	failed := t.ExecutionsCountByResult.Failed
	return fmt.Sprintf("%d %s (%d passed, %d failed)", attempts, plural(attempts, "attempt"), passed, failed)
}

func latestTestExecution(t buildkite.BuildTest) *buildkite.BuildTestExecution {
	executions := testExecutionsInTimestampOrder(t.Executions)
	if len(executions) == 0 {
		return nil
	}

	latest := executions[len(executions)-1]
	if latest.Timestamp == nil {
		return nil
	}

	return &latest
}

func testExecutionsInTimestampOrder(executions []buildkite.BuildTestExecution) []buildkite.BuildTestExecution {
	ordered := append([]buildkite.BuildTestExecution(nil), executions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]

		switch {
		case left.Timestamp == nil && right.Timestamp == nil:
			return false
		case left.Timestamp == nil:
			return true
		case right.Timestamp == nil:
			return false
		default:
			return left.Timestamp.Before(right.Timestamp.Time)
		}
	})

	return ordered
}

func isFailedTestExecution(execution *buildkite.BuildTestExecution) bool {
	if execution == nil {
		return false
	}

	return strings.EqualFold(execution.Status, "failed")
}

func formatTestDetail(text string, colored bool) string {
	if !colored {
		return text
	}

	return "\033[2m" + text + "\033[0m"
}

func formatTestStatusIcon(execution *buildkite.BuildTestExecution, colored bool) string {
	status := ""
	if execution != nil {
		status = execution.Status
	}

	if !colored {
		switch {
		case strings.EqualFold(status, "passed"):
			return "✓"
		case strings.EqualFold(status, "failed"):
			return "✗"
		default:
			return "?"
		}
	}

	switch {
	case strings.EqualFold(status, "passed"):
		return "\033[32m✓\033[0m"
	case strings.EqualFold(status, "failed"):
		return "\033[31m✗\033[0m"
	default:
		return "\033[2m?\033[0m"
	}
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if runewidth.StringWidth(s) <= width {
		return s
	}

	ellipsis := "..."
	remaining := width - runewidth.StringWidth(ellipsis)
	if remaining <= 0 {
		return ellipsis
	}

	leftWidth := remaining / 2
	rightWidth := remaining - leftWidth

	return trimLeftToWidth(s, leftWidth) + ellipsis + trimRightToWidth(s, rightWidth)
}

func trimLeftToWidth(s string, width int) string {
	var b strings.Builder
	currentWidth := 0

	for _, r := range s {
		runeWidth := runewidth.RuneWidth(r)
		if currentWidth+runeWidth > width {
			break
		}
		b.WriteRune(r)
		currentWidth += runeWidth
	}

	return b.String()
}

func trimRightToWidth(s string, width int) string {
	runes := []rune(s)
	currentWidth := 0
	start := len(runes)

	for start > 0 {
		runeWidth := runewidth.RuneWidth(runes[start-1])
		if currentWidth+runeWidth > width {
			break
		}
		currentWidth += runeWidth
		start--
	}

	return string(runes[start:])
}

func padRightToWidth(s string, width int) string {
	padding := width - runewidth.StringWidth(s)
	if padding <= 0 {
		return s
	}

	return s + strings.Repeat(" ", padding)
}
