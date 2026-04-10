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

func (p jobPresenter) failParts(j buildkite.Job) (symbol, name, detail string) {
	job := watch.NewFormattedJob(j)
	name = job.DisplayName()

	var status string
	switch {
	case job.IsSoftFailed():
		status = "soft failed"
	default:
		status = j.State
	}

	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		status += fmt.Sprintf(" with exit %d", *j.ExitStatus)
	}

	symbol = "✗"
	if job.IsSoftFailed() {
		symbol = "⚠"
	}

	detail = fmt.Sprintf("%s — %s", status, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
	return symbol, name, detail
}

func (p jobPresenter) Line(j buildkite.Job) string {
	symbol, name, detail := p.failParts(j)
	return fmt.Sprintf("%s %s %s", symbol, name, detail)
}

func (p jobPresenter) PassedLine(j buildkite.Job) string {
	name := watch.NewFormattedJob(j).DisplayName()
	return fmt.Sprintf("✔ %s", name)
}

func (p jobPresenter) RetryPassedLine(j buildkite.Job) string {
	name := watch.NewFormattedJob(j).DisplayName()
	return fmt.Sprintf("✔ %s passed on retry (attempt %d)", name, j.RetriesCount+1)
}

func (p jobPresenter) ColoredRetryPassedLine(j buildkite.Job) string {
	emojiPrefix, textName := emoji.Split(watch.NewFormattedJob(j).DisplayName())
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	detail := fmt.Sprintf("passed on retry (attempt %d)", j.RetriesCount+1)
	if emojiPrefix != "" {
		return style.Render("✔ ") + emoji.Render(emojiPrefix) + " " + style.Render(fmt.Sprintf("%s %s", textName, detail))
	}
	return style.Render(fmt.Sprintf("✔ %s %s", textName, detail))
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
	symbol, name, detail := p.failParts(j)

	emojiPrefix, textName := emoji.Split(name)

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	if job.IsSoftFailed() {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	}

	if emojiPrefix != "" {
		return style.Render(symbol+" ") + emoji.Render(emojiPrefix) + " " + style.Render(fmt.Sprintf("%s %s", textName, detail))
	}
	return style.Render(fmt.Sprintf("%s %s %s", symbol, textName, detail))
}
