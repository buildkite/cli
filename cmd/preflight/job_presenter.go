package preflight

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type jobPresenter struct {
	pipeline    string
	buildNumber int
}

func (p jobPresenter) Line(j buildkite.Job) string {
	job := watch.NewFormattedJob(j)
	name := job.DisplayName()

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

	symbol := "✗"
	if job.IsSoftFailed() {
		symbol = "⚠"
	}

	return fmt.Sprintf("%s %s %s — %s", symbol, name, status, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
}
