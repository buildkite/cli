package preflight

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
)

type renderer interface {
	appendSnapshotLine(string)
	setSnapshot(*internalpreflight.SnapshotResult)
	renderStatus(watch.BuildStatus, string) error
	flush()
	renderFinalFailures(Result, watch.FailedJobs)
}

func newRenderer(stdout *os.File, tty bool, pipeline string, buildNumber int) renderer {
	if tty {
		return newTTYRenderer(stdout, pipeline, buildNumber)
	}
	return newPlainRenderer(stdout, pipeline, buildNumber)
}

type ttyRenderer struct {
	pipeline       string
	buildNumber    int
	screen         *internalpreflight.Screen
	snapshotRegion *internalpreflight.Region
	titleRegion    *internalpreflight.Region
	failedRegion   *internalpreflight.Region
	jobsRegion     *internalpreflight.Region
	resultRegion   *internalpreflight.Region
}

func newTTYRenderer(stdout *os.File, pipeline string, buildNumber int) *ttyRenderer {
	screen := internalpreflight.NewScreen(stdout)
	return &ttyRenderer{
		pipeline:       pipeline,
		buildNumber:    buildNumber,
		screen:         screen,
		snapshotRegion: screen.AddRegion("snapshot"),
		titleRegion:    screen.AddRegion("title"),
		failedRegion:   screen.AddRegion("failed"),
		jobsRegion:     screen.AddRegion("jobs"),
		resultRegion:   screen.AddRegion("result"),
	}
}

func (r *ttyRenderer) appendSnapshotLine(line string) {
	r.snapshotRegion.AppendLine(line)
}

func (r *ttyRenderer) setSnapshot(result *internalpreflight.SnapshotResult) {
	r.snapshotRegion.SetLines(snapshotLines(result))
}

func (r *ttyRenderer) renderStatus(status watch.BuildStatus, buildState string) error {
	var presenter jobPresenter = ttyJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber}
	title := fmt.Sprintf("  %s Watching build #%d…", spinner(), r.buildNumber)
	if buildState != "" {
		title += " (" + buildState + ")"
	}
	r.titleRegion.SetLines([]string{
		"",
		title,
		"",
	})
	for _, failed := range status.NewlyFailed {
		r.failedRegion.AppendLine(presenter.Line(failed))
	}

	var lines []string
	lines = append(lines, formatSummaryLine(status.Summary))
	r.jobsRegion.SetLines(lines)
	return nil
}

func (r *ttyRenderer) flush() {
	r.screen.Flush()
}

func (r *ttyRenderer) renderFinalFailures(result Result, failedJobs watch.FailedJobs) {
	var presenter jobPresenter = ttyJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber}
	r.resultRegion.SetLines(finalResultLines(result, failedJobs, presenter))
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

func (r *plainRenderer) appendSnapshotLine(line string) {
	fmt.Fprintln(r.stdout, line)
}

func (r *plainRenderer) setSnapshot(result *internalpreflight.SnapshotResult) {
	for _, line := range snapshotLines(result) {
		fmt.Fprintln(r.stdout, line)
	}
}

func (r *plainRenderer) renderStatus(status watch.BuildStatus, buildState string) error {
	var presenter jobPresenter = plainJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber}
	for _, failed := range status.NewlyFailed {
		if _, err := fmt.Fprintf(r.stdout, "%s\n", presenter.Line(failed)); err != nil {
			return err
		}
	}

	line := fmt.Sprintf("Build #%d %s", r.buildNumber, buildState)
	if summary := status.Summary.String(); summary != "" {
		line += " — " + summary
	}
	if line != r.lastLine {
		if _, err := fmt.Fprintf(r.stdout, "[%s] %s\n", time.Now().Format(time.TimeOnly), line); err != nil {
			return err
		}
		r.lastLine = line
	}
	return nil
}

func (r *plainRenderer) flush() {}

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

func (r *plainRenderer) renderFinalFailures(result Result, failedJobs watch.FailedJobs) {
	var presenter jobPresenter = plainJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber, final: true}
	for _, line := range finalResultLines(result, failedJobs, presenter) {
		fmt.Fprintln(r.stdout, line)
	}
}

func finalResultLines(result Result, failedJobs watch.FailedJobs, presenter jobPresenter) []string {
	lines := []string{"", resultStatusLine(result)}

	if len(failedJobs.Hard) > 0 {
		lines = append(lines, "", fmt.Sprintf("Failed jobs (%d):", len(failedJobs.Hard)))
		for _, rawJob := range failedJobs.Hard {
			lines = append(lines, presenter.Line(rawJob))
		}
	}

	if len(failedJobs.Soft) > 0 {
		lines = append(lines, "", fmt.Sprintf("Soft failed jobs (%d):", len(failedJobs.Soft)))
		for _, rawJob := range failedJobs.Soft {
			lines = append(lines, presenter.Line(rawJob))
		}
	}

	lines = append(lines, "")

	return lines
}

func resultStatusLine(result Result) string {
	if result.kind == resultCompletedPass {
		return "✅ Preflight build passed."
	}
	return fmt.Sprintf("❌ Preflight build %s.", result.buildState)
}

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

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinner() string {
	idx := int(time.Now().UnixMilli()/50) % len(spinnerFrames)
	return "\033[36m" + spinnerFrames[idx] + "\033[0m"
}

func jobLogCommand(pipeline string, buildNumber int, jobID string) string {
	return fmt.Sprintf("bk job log -b %d -p %s %s", buildNumber, pipeline, jobID)
}

func formatSummaryLine(s watch.JobSummary) string {
	var parts []string
	if s.Passed > 0 {
		parts = append(parts, fmt.Sprintf("\033[32m%d passed\033[0m", s.Passed))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("\033[31m%d failed\033[0m", s.Failed))
	}
	if s.SoftFailed > 0 {
		parts = append(parts, fmt.Sprintf("\033[33m%d soft failed\033[0m", s.SoftFailed))
	}
	if s.Scheduled > 0 {
		parts = append(parts, fmt.Sprintf("%d scheduled", s.Scheduled))
	}
	if s.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", s.Waiting))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("  … %s", strings.Join(parts, ", "))
}
