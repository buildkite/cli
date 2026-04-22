package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
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
		if line := summaryBuildLine(e); line != "" {
			if _, err := fmt.Fprintf(r.stdout, "  %s\n", line); err != nil {
				return err
			}
		}
		presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
		for _, j := range e.PassedJobs {
			if _, err := fmt.Fprintf(r.stdout, "  %s\n", presenter.PassedLine(j)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(r.stdout, buildSummaryDetails(e, false, 0)); err != nil {
			return err
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

func summaryBuildLine(e Event) string {
	label := summaryBuildLabel(e)
	if e.BuildURL == "" || label == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", label, e.BuildURL)
}

func summaryBuildLabel(e Event) string {
	if e.BuildNumber > 0 {
		return fmt.Sprintf("Build #%d", e.BuildNumber)
	}
	return ""
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

const summaryTestFailureDisplayLimit = 10

func buildSummaryDetails(e Event, colored bool, width int) string {
	var sections []string

	if len(e.FailedJobs) > 0 {
		presenter := jobPresenter{pipeline: e.Pipeline, buildNumber: e.BuildNumber}
		lines := []string{"    Build Failures:"}
		for _, j := range e.FailedJobs {
			line := presenter.Line(j)
			if colored {
				line = presenter.ColoredLine(j)
			}
			lines = append(lines, "        "+line)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	if testSection := summaryTestsSection(e.Tests.Runs, e.Tests.Failures, width); testSection != "" {
		sections = append(sections, testSection)
	}

	if len(sections) == 0 {
		return ""
	}

	return "\n\n" + strings.Join(sections, "\n\n") + "\n"
}

func summaryTestsSection(tests map[string]internalpreflight.SummaryTestRun, failures []internalpreflight.SummaryTestFailure, width int) string {
	if len(tests) == 0 && len(failures) == 0 {
		return ""
	}

	presenter := testPresenter{}
	summaries := orderedSummaryTestRuns(tests)
	header := "    Tests Passed ✓"
	failedTests := 0
	for _, summary := range summaries {
		failedTests += summary.Failed
	}
	totalFailed := max(failedTests, len(failures))
	if totalFailed > 0 {
		header = "    Tests Failed ✗"
	}
	lines := []string{header}

	if len(summaries) > 0 {
		widths := summarySuiteWidths(summaries)
		for _, summary := range summaries {
			lines = append(lines, "        "+presenter.SummarySuiteLine(summary, widths))
		}
	}

	displayed := min(len(failures), summaryTestFailureDisplayLimit)
	if displayed > 0 {
		lines = append(lines, "")
		for _, failure := range failures[:displayed] {
			lines = append(lines, presenter.SummaryFailureLine(failure, width, "        "))
		}
	}

	if remaining := totalFailed - displayed; remaining > 0 {
		lines = append(lines, fmt.Sprintf("        ... and %d more failed %s", remaining, plural(remaining, "test")))
	}

	return strings.Join(lines, "\n")
}

func orderedSummaryTestRuns(tests map[string]internalpreflight.SummaryTestRun) []internalpreflight.SummaryTestRun {
	summaries := make([]internalpreflight.SummaryTestRun, 0, len(tests))
	for _, summary := range tests {
		summaries = append(summaries, summary)
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		leftFailed := summaries[i].Failed > 0
		rightFailed := summaries[j].Failed > 0
		if leftFailed != rightFailed {
			return leftFailed
		}

		leftLabel := strings.ToLower(summarySuiteLabel(summaries[i].SuiteName, summaries[i].SuiteSlug, "unknown"))
		rightLabel := strings.ToLower(summarySuiteLabel(summaries[j].SuiteName, summaries[j].SuiteSlug, "unknown"))
		return leftLabel < rightLabel
	})

	return summaries
}

func summarySuiteWidths(tests []internalpreflight.SummaryTestRun) summarySuiteColumnWidths {
	widths := summarySuiteColumnWidths{Failed: 1, Passed: 1, Skipped: 1}
	for _, summary := range tests {
		widths.Label = max(widths.Label, runewidth.StringWidth(summarySuiteLabel(summary.SuiteName, summary.SuiteSlug, "unknown")))
		widths.Failed = max(widths.Failed, len(strconv.Itoa(summary.Failed)))
		widths.Passed = max(widths.Passed, len(strconv.Itoa(summary.Passed)))
		widths.Skipped = max(widths.Skipped, len(strconv.Itoa(summary.Skipped)))
	}

	return widths
}

func summarySuiteLabel(name, slug, fallback string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}

	if slug = strings.TrimSpace(slug); slug != "" {
		return slug
	}

	return fallback
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
	head, tail, ok := strings.Cut(text, "\n")
	if !ok {
		return prefix + head
	}

	return prefix + head + "\n" + indentAllLines(tail, len(prefix))
}

func indentAllLines(text string, indentWidth int) string {
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", indentWidth)
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
