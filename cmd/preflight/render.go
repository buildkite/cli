package preflight

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
)

type renderer interface {
	Render(Event)
	Close()
}

func newRenderer(stdout io.Writer, jsonMode bool, textMode bool) renderer {
	if jsonMode {
		return newJSONRenderer(stdout)
	}
	if !textMode {
		return newTTYRenderer()
	}
	return newPlainRenderer(stdout)
}

type plainRenderer struct {
	stdout   io.Writer
	lastLine string
}

func newPlainRenderer(stdout io.Writer) *plainRenderer {
	return &plainRenderer{stdout: stdout}
}

func (r *plainRenderer) Render(e Event) {
	switch e.Type {
	case EventStatus:
		if e.Operation != "" {
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), e.Operation)
			return
		}
		line := fmt.Sprintf("Build #%d %s", e.BuildNumber, e.BuildState)
		if e.Jobs != nil {
			if summary := e.Jobs.String(); summary != "" {
				line += " — " + summary
			}
		}
		if line != r.lastLine {
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), line)
			r.lastLine = line
		}

	case EventJobFailure:
		if e.Job != nil {
			presenter := plainJobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
			fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), presenter.Line(*e.Job))
		}
	}
}

func (r *plainRenderer) Close() {}

type jsonRenderer struct {
	encoder *json.Encoder
}

func newJSONRenderer(stdout io.Writer) *jsonRenderer {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return &jsonRenderer{encoder: enc}
}

func (r *jsonRenderer) Render(e Event) {
	r.encoder.Encode(e)
}

func (r *jsonRenderer) Close() {}

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
