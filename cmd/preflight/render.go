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
	"github.com/mattn/go-runewidth"

	buildkite "github.com/buildkite/go-buildkite/v4"
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
			sep := "\n"
			detail := indentAllLines(e.Detail, len("["+time.TimeOnly+"] "))
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
			for _, t := range e.TestFailures {
				if _, err := fmt.Fprintf(r.stdout, "[%s] %s\n", e.Time.Format(time.TimeOnly), formatTestFailureLine(t)); err != nil {
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

func indentAllLines(text string, indentWidth int) string {
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", indentWidth)
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func formatTestFailureLine(t buildkite.BuildTest) string {
	name := t.Name
	if t.Scope != "" {
		name = t.Scope + " " + name
	}
	name = truncateToWidth(name, 80)

	history := formatTestExecutionHistory(t)
	line := fmt.Sprintf("  %s \033[33mtest:\033[0m %s", history, name)

	currentExecution, ok := latestTestExecution(t)
	if !ok || !isFailedTestExecution(currentExecution) {
		return line
	}

	if location := currentExecution.Location; location != "" {
		line += fmt.Sprintf("\n    \033[2mLocation: %s\033[0m", location)
	} else if t.Location != "" {
		line += fmt.Sprintf("\n    \033[2mLocation: %s\033[0m", t.Location)
	}

	if currentExecution.FailureReason != "" {
		line += fmt.Sprintf("\n    \033[2m%s\033[0m", currentExecution.FailureReason)
	}
	for _, fe := range currentExecution.FailureExpanded {
		for _, exp := range fe.Expanded {
			line += fmt.Sprintf("\n    \033[2m%s\033[0m", exp)
		}
		for _, bt := range fe.Backtrace {
			line += fmt.Sprintf("\n    \033[2m%s\033[0m", bt)
		}
	}

	return line
}

func formatTestExecutionHistory(t buildkite.BuildTest) string {
	executions := t.Executions
	if len(executions) == 0 {
		return formatTestStatusIcon(t.State)
	}

	icons := make([]string, 0, len(executions))
	for _, execution := range executions {
		icons = append(icons, formatTestStatusIcon(execution.Status))
	}
	return strings.Join(icons, " ")
}

func latestTestExecution(t buildkite.BuildTest) (buildkite.BuildTestExecution, bool) {
	if len(t.Executions) > 0 {
		return t.Executions[len(t.Executions)-1], true
	}

	if t.State == "" {
		return buildkite.BuildTestExecution{}, false
	}

	return buildkite.BuildTestExecution{
		Status:        t.State,
		Location:      t.Location,
		FailureReason: latestTestFailureReason(t),
	}, true
}

func latestTestFailureReason(t buildkite.BuildTest) string {
	if t.LatestFail == nil {
		return ""
	}
	return t.LatestFail.FailureReason
}

func isFailedTestExecution(execution buildkite.BuildTestExecution) bool {
	return strings.EqualFold(execution.Status, "failed")
}

func formatTestStatusIcon(status string) string {
	switch {
	case strings.EqualFold(status, "passed"):
		return "\033[32m✅\033[0m"
	case strings.EqualFold(status, "failed"):
		return "\033[31m❌\033[0m"
	default:
		return "\033[2m?\033[0m"
	}
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if runewidth.StringWidth(s) <= width {
		return s
	}

	if width <= 1 {
		return "…"
	}

	var b strings.Builder
	currentWidth := 0

	for _, r := range s {
		runeWidth := runewidth.RuneWidth(r)
		if currentWidth+runeWidth > width-1 {
			break
		}
		b.WriteRune(r)
		currentWidth += runeWidth
	}

	b.WriteRune('…')
	return b.String()
}
