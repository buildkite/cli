package preflight

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/build/watch"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

const (
	ttyFailedLabelWidth    = 50
	ttyFailedStateWidth    = 11
	ttyFailedDurationWidth = 8
	ttyFailedExitWidth     = 8
)

type jobPresenter interface {
	Line(buildkite.Job) string
}

type ttyJobPresenter struct {
	pipeline    string
	buildNumber int
}

func (p ttyJobPresenter) Line(j buildkite.Job) string {
	job := watch.NewFormattedJob(j)
	if j.State == "running" || j.State == "canceling" || j.State == "timing_out" {
		durationString := ""
		if duration := job.Duration(); duration > 0 {
			durationString = "\033[2m " + duration.String() + "\033[0m"
		}
		return fmt.Sprintf("  \033[36m●\033[0m %-50s \033[36mrunning\033[0m%s", job.DisplayName(), durationString)
	}
	stateLabel := fmt.Sprintf("\033[31m%s\033[0m", j.State)
	if job.IsSoftFailed() {
		stateLabel = "\033[33msoft failed\033[0m"
	}

	durationString := "-"
	if duration := job.Duration(); duration > 0 {
		durationString = duration.String()
	}

	exit := "-"
	if j.ExitStatus != nil && *j.ExitStatus != 0 {
		exit = fmt.Sprintf("exit %d", *j.ExitStatus)
	}

	exitLabel := fmt.Sprintf("\033[2m%*s\033[0m", ttyFailedExitWidth, exit)
	if exit != "-" {
		exitLabel = fmt.Sprintf("\033[31m%*s\033[0m", ttyFailedExitWidth, exit)
	}

	return fmt.Sprintf(
		"  \033[31m✗\033[0m %-*s %s \033[2m%*s\033[0m %s \033[2m%s\033[0m \033[2m(%s)\033[0m",
		ttyFailedLabelWidth, job.DisplayName(),
		padDisplayRight(stateLabel, ttyFailedStateWidth),
		ttyFailedDurationWidth, durationString,
		exitLabel,
		j.ID,
		jobLogCommand(p.pipeline, p.buildNumber, j.ID),
	)
}

type plainJobPresenter struct {
	pipeline    string
	buildNumber int
	final       bool
}

func (p plainJobPresenter) Line(j buildkite.Job) string {
	job := watch.NewFormattedJob(j)
	if p.final {
		if job.IsSoftFailed() {
			return fmt.Sprintf("  ⚠ %s  %s  (%s)", job.DisplayName(), j.ID, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
		}
		return fmt.Sprintf("  ✗ %s  %s  %s  (%s)", job.DisplayName(), j.State, j.ID, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
	}
	if job.IsSoftFailed() {
		return fmt.Sprintf("  ⚠ %s  soft failed  %s  (%s)", job.DisplayName(), j.ID, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
	}
	return fmt.Sprintf("  ✗ %s  %s  %s  (%s)", job.DisplayName(), j.State, j.ID, jobLogCommand(p.pipeline, p.buildNumber, j.ID))
}

func padDisplayRight(s string, width int) string {
	padding := width - len(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "\033[31m", ""), "\033[33m", ""), "\033[0m", ""))
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}
