package preflight

import (
	"fmt"
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
	executions := t.Executions
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
	var latest *buildkite.BuildTestExecution
	for i := range t.Executions {
		execution := &t.Executions[i]
		if execution.Timestamp == nil {
			continue
		}
		if latest == nil || execution.Timestamp.After(latest.Timestamp.Time) {
			latest = execution
		}
	}

	return latest
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

	if width <= 1 {
		return "…"
	}

	var b strings.Builder
	currentWidth := 0

	for _, r := range s {
		runeWidth := runewidth.RuneWidth(r)
		if currentWidth+runeWidth > width-1 {
			break
		}
		b.WriteRune(r)
		currentWidth += runeWidth
	}

	b.WriteRune('…')
	return b.String()
}
