package preflight

import (
	"fmt"
	"io"
	"time"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
)

type renderer interface {
	Render(Event)
	Close()
}

func newRenderer(stdout io.Writer, pipeline string, buildNumber int) renderer {
	return newPlainRenderer(stdout, pipeline, buildNumber)
}

type plainRenderer struct {
	pipeline    string
	buildNumber int
	stdout      io.Writer
	lastLine    string
}

func newPlainRenderer(stdout io.Writer, pipeline string, buildNumber int) *plainRenderer {
	return &plainRenderer{stdout: stdout, pipeline: pipeline, buildNumber: buildNumber}
}

func (r *plainRenderer) Render(e Event) {
	presenter := plainJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber}

	switch e.Type {
	case EventStatus:
		if e.Operation != "" {
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), e.Operation)
			return
		}
		line := fmt.Sprintf("Build #%d %s", e.BuildNumber, e.BuildState)
		if summary := e.Jobs.String(); summary != "" {
			line += " — " + summary
		}
		if line != r.lastLine {
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), line)
			r.lastLine = line
		}

	case EventJobFailure:
		if e.Job != nil {
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), presenter.Line(*e.Job))
		}
	}
}

func (r *plainRenderer) Close() {}

func snapshotLines(result *internalpreflight.SnapshotResult) []string {
	lines := []string{
		fmt.Sprintf("Commit: %s", result.Commit[:10]),
		fmt.Sprintf("Ref:    %s", result.Ref),
	}
	if len(result.Files) > 0 {
		lines = append(lines, fmt.Sprintf("Files:  %d changed", len(result.Files)))
		for _, file := range result.Files {
			lines = append(lines, fmt.Sprintf("  %s %s", file.StatusSymbol(), file.Path))
		}
	}
	return lines
}

func jobLogCommand(pipeline string, buildNumber int, jobID string) string {
	return fmt.Sprintf("bk job log -b %d -p %s %s", buildNumber, pipeline, jobID)
}
