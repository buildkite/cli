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

// failParts returns the shared components of a failed/soft-failed job line.
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

// Line renders a failed/soft-failed job for plain (uncoloured) output.
func (p jobPresenter) Line(j buildkite.Job) string {
	symbol, name, detail := p.failParts(j)
	return fmt.Sprintf("%s %s %s", symbol, emoji.Render(name), detail)
}

// PassedLine renders a passed job for plain (uncoloured) output.
func (p jobPresenter) PassedLine(j buildkite.Job) string {
	name := emoji.Render(watch.NewFormattedJob(j).DisplayName())
	return fmt.Sprintf("✔ %s", name)
}

// ColoredLine renders a failed/soft-failed job with lipgloss foreground
// colour. The leading emoji is rendered outside the colour span so that
// Kitty graphics placeholder colour resets don't break the styling.
func (p jobPresenter) ColoredLine(j buildkite.Job) string {
	job := watch.NewFormattedJob(j)
	symbol, name, detail := p.failParts(j)

	emojiPrefix, textName := emoji.Split(name)

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	if job.IsSoftFailed() {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	}

	colored := style.Render(fmt.Sprintf("%s %s %s", symbol, textName, detail))
	if emojiPrefix != "" {
		return emoji.Render(emojiPrefix) + " " + colored
	}
	return colored
}
