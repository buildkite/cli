package preflight

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type testPresenter struct{}

type testOutcome int

const (
	testOutcomeUnknown testOutcome = iota
	testOutcomePassed
	testOutcomeFailed
	testOutcomeMixed
)

func (p testPresenter) Line(t buildkite.BuildTest) string {
	return p.line(t, false)
}

func (p testPresenter) ColoredLine(t buildkite.BuildTest) string {
	return p.line(t, true)
}

func (p testPresenter) ttyBlock(t buildkite.BuildTest) string {
	name := t.Name
	if t.Scope != "" {
		name = t.Scope + " " + name
	}
	name = truncateToWidth(name, 80)

	latestExecution := latestTestExecution(t)
	displayOutcome := testDisplayOutcome(t, latestExecution)
	headline := ttyTestLabelStyle(displayOutcome).Render("● test") + " " + ttyTitleStyle.Render(name)
	if badge := ttyTestFailureBadge(t); badge != "" {
		headline += " " + badge
	}
	lines := []string{headline}

	if location := testLocation(t, latestExecution); location != "" {
		lines = append(lines, ttyContinuationLines(location, ttyDimStyle)...)
	}
	if executionSummary := testExecutionSummary(t); executionSummary != "" {
		lines = append(lines, ttyContinuationLines(executionSummary, ttyDimStyle)...)
	}
	if latestExecution != nil && isFailedTestExecution(latestExecution) && latestExecution.FailureReason != "" {
		lines = append(lines, ttyContinuationLines(latestExecution.FailureReason, lipgloss.NewStyle())...)
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
	displayOutcome := testDisplayOutcome(t, latestExecution)

	statusIcon := formatTestStatusIcon(displayOutcome, colored)
	line := fmt.Sprintf("%s %s", statusIcon, name)
	if badge := testFailureBadge(t, colored); badge != "" {
		line += " " + badge
	}

	detailParts := make([]string, 0, 2)
	if executionSummary := testExecutionSummary(t); executionSummary != "" {
		detailParts = append(detailParts, executionSummary)
	}
	if location := testLocation(t, latestExecution); location != "" {
		detailParts = append(detailParts, location)
	}
	if len(detailParts) > 0 {
		line += fmt.Sprintf("\n    %s", formatTestDetail(strings.Join(detailParts, " — "), colored))
	}

	if latestExecution != nil && isFailedTestExecution(latestExecution) && latestExecution.FailureReason != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(latestExecution.FailureReason, colored))
	}

	return line
}

func testExecutionSummary(t buildkite.BuildTest) string {
	totalExecutions := testExecutionCount(t)
	if totalExecutions == 0 {
		return ""
	}

	passed := t.ExecutionsCountByResult.Passed
	failed := t.ExecutionsCountByResult.Failed

	switch testOutcomeStatus(t) {
	case testOutcomeFailed:
		return fmt.Sprintf("%d failed %s", failed, plural(failed, "execution"))
	case testOutcomePassed:
		return fmt.Sprintf("%d passed %s", passed, plural(passed, "execution"))
	case testOutcomeMixed:
		return fmt.Sprintf("%d %s · %d passed, %d failed", totalExecutions, plural(totalExecutions, "execution"), passed, failed)
	default:
		return fmt.Sprintf("%d %s", totalExecutions, plural(totalExecutions, "execution"))
	}
}

func testFailureBadge(t buildkite.BuildTest, colored bool) string {
	failed := t.ExecutionsCountByResult.Failed
	if failed <= 1 || testOutcomeStatus(t) != testOutcomeFailed {
		return ""
	}

	text := fmt.Sprintf("[%d failed]", failed)
	if !colored {
		return text
	}

	return "\033[31;1m" + text + "\033[0m"
}

func ttyTestFailureBadge(t buildkite.BuildTest) string {
	failed := t.ExecutionsCountByResult.Failed
	if failed <= 1 || testOutcomeStatus(t) != testOutcomeFailed {
		return ""
	}

	text := fmt.Sprintf("[%d failed]", failed)
	return ttyFailureStyle.Bold(true).Render(text)
}

func testExecutionCount(t buildkite.BuildTest) int {
	if t.ExecutionsCount > 0 {
		return t.ExecutionsCount
	}

	counts := t.ExecutionsCountByResult
	return counts.Passed + counts.Failed + counts.Skipped + counts.Pending + counts.Unknown
}

func testOutcomeStatus(t buildkite.BuildTest) testOutcome {
	totalExecutions := testExecutionCount(t)
	if totalExecutions == 0 {
		return testOutcomeUnknown
	}

	counts := t.ExecutionsCountByResult
	switch {
	case counts.Failed > 0 && counts.Failed == totalExecutions:
		return testOutcomeFailed
	case counts.Passed > 0 && counts.Passed == totalExecutions:
		return testOutcomePassed
	case counts.Passed > 0 && counts.Failed > 0:
		return testOutcomeMixed
	default:
		return testOutcomeUnknown
	}
}

func testDisplayOutcome(t buildkite.BuildTest, latestExecution *buildkite.BuildTestExecution) testOutcome {
	outcome := testOutcomeStatus(t)
	if outcome == testOutcomeMixed && latestExecution != nil && strings.EqualFold(latestExecution.Status, "passed") {
		return testOutcomePassed
	}
	return outcome
}

func testLocation(t buildkite.BuildTest, latestExecution *buildkite.BuildTestExecution) string {
	if latestExecution != nil && latestExecution.Location != "" {
		return latestExecution.Location
	}
	return t.Location
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

func formatTestStatusIcon(outcome testOutcome, colored bool) string {
	if !colored {
		switch outcome {
		case testOutcomePassed:
			return "✓"
		case testOutcomeFailed:
			return "✗"
		case testOutcomeMixed:
			return "!"
		default:
			return "?"
		}
	}

	switch outcome {
	case testOutcomePassed:
		return "\033[32m✓\033[0m"
	case testOutcomeFailed:
		return "\033[31m✗\033[0m"
	case testOutcomeMixed:
		return "\033[33m!\033[0m"
	default:
		return "\033[2m?\033[0m"
	}
}

func ttyTestLabelStyle(outcome testOutcome) lipgloss.Style {
	switch outcome {
	case testOutcomePassed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	case testOutcomeFailed:
		return ttyFailureStyle.Bold(true)
	case testOutcomeMixed:
		return ttySoftFailureStyle.Bold(true)
	default:
		return ttyTestStyle
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
