package preflight

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/build/watch"
	"github.com/buildkite/cli/v3/internal/emoji"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

type jobPresenter struct {
	pipeline    string
	buildNumber int
}

func (p jobPresenter) failParts(j buildkite.Job) (symbol, name, status, logCommand string) {
	job := watch.NewFormattedJob(j)
	name = job.DisplayName()

	status = j.State
	switch {
	case job.IsSoftFailed():
		status = "soft failed"
	}

	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		status += fmt.Sprintf(" with exit %d", *j.ExitStatus)
	}

	symbol = "✗"
	if job.IsSoftFailed() {
		symbol = "⚠"
	}

	logCommand = jobLogCommand(p.pipeline, p.buildNumber, j.ID)
	return symbol, name, status, logCommand
}

func (p jobPresenter) Line(j buildkite.Job) string {
	symbol, name, status, logCommand := p.failParts(j)
	return fmt.Sprintf("%s %s %s — %s", symbol, name, status, logCommand)
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
	job := watch.NewFormattedJob(j)
	symbol, name, status, _ := p.failParts(j)

	emojiPrefix, textName := emoji.Split(name)

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	if job.IsSoftFailed() {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	}

	if emojiPrefix != "" {
		return style.Render(symbol+" ") + emoji.Render(emojiPrefix) + " " + style.Render(fmt.Sprintf("%s %s", textName, status))
	}
	return style.Render(fmt.Sprintf("%s %s %s", symbol, textName, status))
}
