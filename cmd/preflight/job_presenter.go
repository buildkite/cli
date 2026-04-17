package preflight

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/build/watch"
	"github.com/buildkite/cli/v3/internal/emoji"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

type jobPresenter struct {
	pipeline    string
	buildNumber int
}

func (p jobPresenter) failParts(j buildkite.Job) (symbol, name, status, command string, softFailed bool) {
	job := watch.NewFormattedJob(j)
	name = job.DisplayName()
	softFailed = job.IsSoftFailed()

	switch {
	case softFailed:
		status = "soft failed"
	default:
		status = j.State
	}

	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		status += fmt.Sprintf(" with exit %d", *j.ExitStatus)
	}

	symbol = "✗"
	if softFailed {
		symbol = "⚠"
	}

	command = jobLogCommand(p.pipeline, p.buildNumber, j.ID)
	return symbol, name, status, command, softFailed
}

func (p jobPresenter) Line(j buildkite.Job) string {
	symbol, name, status, command, _ := p.failParts(j)
	return fmt.Sprintf("%s %s %s — %s", symbol, name, status, command)
}

func (p jobPresenter) PassedLine(j buildkite.Job) string {
	name := watch.NewFormattedJob(j).DisplayName()
	return fmt.Sprintf("✔ %s", name)
}

func (p jobPresenter) ColoredPassedLine(j buildkite.Job, style lipgloss.Style) string {
	emojiPrefix, textName := emoji.Split(watch.NewFormattedJob(j).DisplayName())
	if emojiPrefix != "" {
		return style.Render("✔ ") + emoji.Render(emojiPrefix) + " " + style.Render(textName)
	}
	return style.Render(fmt.Sprintf("✔ %s", textName))
}

// ColoredLine renders emoji outside the ANSI colour span to avoid
// Kitty/iTerm2 graphics escape sequences breaking lipgloss styling.
func (p jobPresenter) ColoredLine(j buildkite.Job) string {
	symbol, name, status, command, softFailed := p.failParts(j)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	if softFailed {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	}

	return style.Render(symbol+" ") + renderDisplayName(name, style) + style.Render(fmt.Sprintf(" %s — %s", status, command))
}

func (p jobPresenter) ttyBlock(j buildkite.Job) string {
	_, name, status, command, softFailed := p.failParts(j)

	labelStyle := ttyFailureStyle.Bold(true)
	statusStyle := ttyFailureStyle
	if softFailed {
		labelStyle = ttySoftFailureStyle.Bold(true)
		statusStyle = ttySoftFailureStyle
	}

	lines := []string{
		labelStyle.Render("● job") + " " + renderDisplayName(name, ttyTitleStyle),
	}
	lines = append(lines, ttyContinuationLines(status, statusStyle)...)
	lines = append(lines, ttyContinuationLines(command, ttyCommandStyle)...)

	return strings.Join(lines, "\n")
}

func renderDisplayName(name string, style lipgloss.Style) string {
	emojiPrefix, textName := emoji.Split(name)
	if emojiPrefix == "" {
		return style.Render(textName)
	}
	if textName == "" {
		return emoji.Render(emojiPrefix)
	}
	return emoji.Render(emojiPrefix) + " " + style.Render(textName)
}
