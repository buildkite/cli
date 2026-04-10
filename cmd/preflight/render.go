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
	prefix := timestampPrefix(e.Time)

	switch e.Type {
	case EventOperation:
		if e.Detail != "" {
			_, err := fmt.Fprintf(r.stdout, "%s\n", formatTimestampedDetail(e.Title, e.Detail, e.Time))
			return err
		}
		_, err := fmt.Fprintf(r.stdout, "%s%s\n", prefix, e.Title)
		return err

	case EventBuildStatus:
		line := fmt.Sprintf("Build #%d %s", e.BuildNumber, e.BuildState)
		if e.Jobs != nil {
			if summary := e.Jobs.String(); summary != "" {
				line += " — " + summary
			}
		}
		if line != r.lastLine {
			_, err := fmt.Fprintf(r.stdout, "%s%s\n", prefix, line)
			r.lastLine = line
			return err
		}

	case EventJobFailure:
		if e.Job != nil {
			presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
			_, err := fmt.Fprintf(r.stdout, "%s%s\n", prefix, presenter.Line(*e.Job))
			return err
		}

	case EventJobRetryPassed:
		if e.Job != nil {
			presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
			_, err := fmt.Fprintf(r.stdout, "%s%s\n", prefix, presenter.RetryPassedLine(*e.Job))
			return err
		}

	case EventBuildSummary:
		header := summaryHeader(e)
		if _, err := fmt.Fprintf(r.stdout, "\n%s\n", header); err != nil {
			return err
		}
		presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
		for _, j := range e.PassedJobs {
			if _, err := fmt.Fprintf(r.stdout, "  %s\n", presenter.PassedLine(j)); err != nil {
				return err
			}
		}
		for _, j := range e.FailedJobs {
			if _, err := fmt.Fprintf(r.stdout, "  %s\n", presenter.Line(j)); err != nil {
				return err
			}
		}

	case EventTestFailure:
		if e.TestFailures != nil {
			presenter := testPresenter{}
			for _, t := range e.TestFailures {
				if _, err := fmt.Fprintf(r.stdout, "%s\n", formatTimestampedBlock(presenter.Line(t), e.Time)); err != nil {
					return err
				}
			}
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

func summaryHeader(e Event) string {
	verdict := "❌ Preflight Failed"
	if e.BuildState == "passed" {
		verdict = "✅ Preflight Passed"
	}
	if e.Duration > 0 {
		return fmt.Sprintf("%s (%s)", verdict, formatDuration(e.Duration))
	}
	return verdict
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d %s %d %s", h, plural(h, "hour"), m, plural(m, "minute"))
	case h > 0:
		return fmt.Sprintf("%d %s", h, plural(h, "hour"))
	case m > 0 && s > 0:
		return fmt.Sprintf("%d %s %d %s", m, plural(m, "minute"), s, plural(s, "second"))
	case m > 0:
		return fmt.Sprintf("%d %s", m, plural(m, "minute"))
	default:
		return fmt.Sprintf("%d %s", s, plural(s, "second"))
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func jobLogCommand(pipeline string, buildNumber int, jobID string) string {
	return fmt.Sprintf("bk job log -b %d -p %s %s", buildNumber, pipeline, jobID)
}

func timestampPrefix(t time.Time) string {
	return t.Format(time.TimeOnly) + " "
}

func formatTimestampedDetail(title, detail string, t time.Time) string {
	return formatTimestampedBlock(title+":\n"+detail, t)
}

func formatTimestampedBlock(text string, t time.Time) string {
	prefix := timestampPrefix(t)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return prefix
	}

	if len(lines) == 1 {
		return prefix + lines[0]
	}

	return prefix + lines[0] + "\n" + indentAllLines(strings.Join(lines[1:], "\n"), len(prefix))
}

func indentAllLines(text string, indentWidth int) string {
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", indentWidth)
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
