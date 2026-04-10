package preflight

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type testPresenter struct{}

func (p testPresenter) Line(t buildkite.BuildTest) string {
	return p.line(t, false)
}

func (p testPresenter) ColoredLine(t buildkite.BuildTest) string {
	return p.line(t, true)
}

func (p testPresenter) line(t buildkite.BuildTest, colored bool) string {
	name := t.Name
	if t.Scope != "" {
		name = t.Scope + " " + name
	}
	name = truncateToWidth(name, 80)

	line := fmt.Sprintf("%s %s", formatTestStatusSummary(t, colored), name)

	latestFailedExecution := latestFailedTestExecution(t)
	if latestFailedExecution == nil {
		return line
	}

	if location := latestFailedExecution.Location; location != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(location, colored))
	} else if t.Location != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(t.Location, colored))
	}

	if latestFailedExecution.FailureReason != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(latestFailedExecution.FailureReason, colored))
	}

	return line
}

func formatTestStatusSummary(t buildkite.BuildTest, colored bool) string {
	passed := t.ExecutionsCountByResult.Passed
	failed := t.ExecutionsCountByResult.Failed
	if colored {
		return fmt.Sprintf("%s %d %s %d", formatTestStatusSymbol("✗", "31", colored), failed, formatTestStatusSymbol("✓", "32", colored), passed)
	}

	return fmt.Sprintf("%d %s, %d %s", failed, testCountWord(failed, "failure", "failures"), passed, testCountWord(passed, "pass", "passes"))
}

func testCountWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}

	return plural
}

func latestFailedTestExecution(t buildkite.BuildTest) *buildkite.BuildTestExecution {
	executions := testExecutionsInTimestampOrder(t.Executions)
	for i := len(executions) - 1; i >= 0; i-- {
		if strings.EqualFold(executions[i].Status, "failed") {
			latest := executions[i]
			return &latest
		}
	}

	return nil
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

func formatTestDetail(text string, colored bool) string {
	if !colored {
		return text
	}

	return "\033[2m" + text + "\033[0m"
}

func formatTestStatusSymbol(symbol, color string, colored bool) string {
	if !colored {
		return symbol
	}

	return fmt.Sprintf("\033[%sm%s\033[0m", color, symbol)
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
