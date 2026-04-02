package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
)

type renderer interface {
	Render(Event) error
	Close() error
}

func newRenderer(stdout io.Writer, jsonMode bool, textMode bool, cancel context.CancelFunc) renderer {
	if jsonMode {
		return newJSONRenderer(stdout)
	}
	isTTY := false
	if f, ok := stdout.(*os.File); ok {
		isTTY = isatty.IsTerminal(f.Fd())
	}
	if textMode || !isTTY {
		return newPlainRenderer(stdout)
	}
	return newTTYRenderer(cancel)
}

type plainRenderer struct {
	stdout   io.Writer
	lastLine string
}

func newPlainRenderer(stdout io.Writer) *plainRenderer {
	return &plainRenderer{stdout: stdout}
}

func (r *plainRenderer) Render(e Event) error {
	switch e.Type {
	case EventOperation:
		if e.Detail != "" {
			sep := " "
			if strings.Contains(e.Detail, "\n") {
				sep = "\n"
			}
			detail := e.Detail
			if sep == "\n" {
				detail = indentAllLines(detail, len("["+time.TimeOnly+"] "))
			}
			_, err := fmt.Fprintf(r.stdout, "[%s] %s:%s%s\n", e.Time.Format(time.TimeOnly), e.Title, sep, detail)
			return err
		}
		_, err := fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), e.Title)
		return err

	case EventBuildStatus:
		line := fmt.Sprintf("Build #%d %s", e.BuildNumber, e.BuildState)
		if e.Jobs != nil {
			if summary := e.Jobs.String(); summary != "" {
				line += " — " + summary
			}
		}
		if line != r.lastLine {
			_, err := fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), line)
			r.lastLine = line
			return err
		}

	case EventJobFailure:
		if e.Job != nil {
			presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
			_, err := fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), presenter.Line(*e.Job))
			return err
		}
	}
	return nil
}

func (r *plainRenderer) Close() error { return nil }

type jsonRenderer struct {
	encoder *json.Encoder
}

func newJSONRenderer(stdout io.Writer) *jsonRenderer {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return &jsonRenderer{encoder: enc}
}

func (r *jsonRenderer) Render(e Event) error {
	return r.encoder.Encode(e)
}

func (r *jsonRenderer) Close() error { return nil }

func jobLogCommand(pipeline string, buildNumber int, jobID string) string {
	return fmt.Sprintf("bk job log -b %d -p %s %s", buildNumber, pipeline, jobID)
}

func indentAllLines(text string, indentWidth int) string {
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", indentWidth)
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
