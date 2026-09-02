package preflight

import (
	"fmt"
	"strings"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

type testPresenter struct{}

type summarySuiteColumnWidths struct {
	Label   int
	Failed  int
	Passed  int
	Skipped int
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
