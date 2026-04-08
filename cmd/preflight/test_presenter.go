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

	history := formatTestExecutionHistory(t, colored)
	line := fmt.Sprintf("%s %s", history, name)

	currentExecution := latestTestExecution(t)
	if !isFailedTestExecution(currentExecution) {
		return line
	}

	if location := currentExecution.Location; location != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail("Location: "+location, colored))
	} else if t.Location != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail("Location: "+t.Location, colored))
	}

	if currentExecution.FailureReason != "" {
		line += fmt.Sprintf("\n    %s", formatTestDetail(currentExecution.FailureReason, colored))
	}

	return line
}

func formatTestExecutionHistory(t buildkite.BuildTest, colored bool) string {
	executions := testExecutionsInTimestampOrder(t.Executions)
	if len(executions) == 0 {
		return formatTestStatusIcon(t.State, colored)
	}

	icons := make([]string, 0, len(executions))
	for _, execution := range executions {
		icons = append(icons, formatTestStatusIcon(execution.Status, colored))
	}
	return strings.Join(icons, " ")
}

func latestTestExecution(t buildkite.BuildTest) *buildkite.BuildTestExecution {
	executions := testExecutionsInTimestampOrder(t.Executions)
	for i := len(executions) - 1; i >= 0; i-- {
		execution := &executions[i]
		if execution.Timestamp != nil {
			return execution
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

func formatTestStatusIcon(status string, colored bool) string {
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

	segmentWidth := width / 2
	if segmentWidth == 0 {
		return "..."
	}

	return trimLeftToWidth(s, segmentWidth) + "..." + trimRightToWidth(s, segmentWidth)
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
