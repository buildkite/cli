package preflight

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type renderer interface {
	appendSnapshotLine(string)
	setSnapshot(*internalpreflight.SnapshotResult)
	renderStatus(watch.BuildStatus, buildkite.Build) error
	flush()
	renderFinalFailures([]buildkite.Job)
	setResult(string)
}

func newRenderer(stdout *os.File, tty bool, pipeline string) renderer {
	if tty {
		return newTTYRenderer(stdout)
	}
	return newPlainRenderer(stdout, pipeline)
}

type ttyRenderer struct {
	screen         *internalpreflight.Screen
	snapshotRegion *internalpreflight.Region
	titleRegion    *internalpreflight.Region
	resultRegion   *internalpreflight.Region
}

func newTTYRenderer(stdout *os.File) *ttyRenderer {
	screen := internalpreflight.NewScreen(stdout)
	return &ttyRenderer{
		screen:         screen,
		snapshotRegion: screen.AddRegion("snapshot"),
		titleRegion:    screen.AddRegion("title"),
		resultRegion:   screen.AddRegion("result"),
	}
}

func (r *ttyRenderer) appendSnapshotLine(line string) {
	r.snapshotRegion.AppendLine(line)
}

func (r *ttyRenderer) setSnapshot(result *internalpreflight.SnapshotResult) {
	r.snapshotRegion.SetLines(snapshotLines(result))
}

func (r *ttyRenderer) renderStatus(status watch.BuildStatus, b buildkite.Build) error {
	line := fmt.Sprintf("  %s Watching build #%d…", spinner(), b.Number)
	if summary := formatSummaryLine(status.Summary); summary != "" {
		line += " " + summary
	}

	r.titleRegion.SetLines([]string{
		"",
		line,
		"",
	})
	return nil
}

func (r *ttyRenderer) flush() {
	r.screen.Flush()
}

func (r *ttyRenderer) renderFinalFailures(_ []buildkite.Job) {}

func (r *ttyRenderer) setResult(state string) {
	if state == "passed" {
		r.resultRegion.SetLines([]string{"", "✅ Preflight passed!"})
		return
	}
	r.resultRegion.SetLines([]string{"", fmt.Sprintf("❌ Preflight %s", state)})
}

type plainRenderer struct {
	pipeline    string
	buildNumber int
	stdout      io.Writer
	lastLine    string
}

func newPlainRenderer(stdout io.Writer, pipeline string) *plainRenderer {
	return &plainRenderer{stdout: stdout, pipeline: pipeline}
}

func (r *plainRenderer) appendSnapshotLine(line string) {
	fmt.Fprintln(r.stdout, line)
}

func (r *plainRenderer) setSnapshot(result *internalpreflight.SnapshotResult) {
	for _, line := range snapshotLines(result) {
		fmt.Fprintln(r.stdout, line)
	}
}

func (r *plainRenderer) renderStatus(status watch.BuildStatus, b buildkite.Build) error {
	r.buildNumber = b.Number
	var presenter jobPresenter = plainJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber}
	for _, failed := range status.NewlyFailed {
		if _, err := fmt.Fprintf(r.stdout, "%s\n", presenter.Line(failed)); err != nil {
			return err
		}
	}

	line := fmt.Sprintf("Build #%d %s", b.Number, b.State)
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

func (r *plainRenderer) renderFinalFailures(allFailed []buildkite.Job) {
	var hardFailed, softFailed []buildkite.Job
	var presenter jobPresenter = plainJobPresenter{pipeline: r.pipeline, buildNumber: r.buildNumber, final: true}
	for _, rawJob := range allFailed {
		job := watch.NewFormattedJob(rawJob)
		if job.IsSoftFailed() {
			softFailed = append(softFailed, rawJob)
		} else {
			hardFailed = append(hardFailed, rawJob)
		}
	}

	if len(hardFailed) > 0 {
		fmt.Fprintln(r.stdout)
		fmt.Fprintf(r.stdout, "Failed jobs (%d):\n", len(hardFailed))
		for _, rawJob := range hardFailed {
			fmt.Fprintf(r.stdout, "%s\n", presenter.Line(rawJob))
		}
	}

	if len(softFailed) > 0 {
		fmt.Fprintln(r.stdout)
		fmt.Fprintf(r.stdout, "Soft failed jobs (%d):\n", len(softFailed))
		for _, rawJob := range softFailed {
			fmt.Fprintf(r.stdout, "%s\n", presenter.Line(rawJob))
		}
	}
}

func (r *plainRenderer) setResult(state string) {
	if state == "passed" {
		fmt.Fprintln(r.stdout, "✅ Preflight passed!")
		return
	}
	fmt.Fprintf(r.stdout, "❌ Preflight %s\n", state)
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
	if s.Running > 0 {
		parts = append(parts, fmt.Sprintf("\033[36m%d running\033[0m", s.Running))
	}
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
	if s.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", s.Blocked))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("jobs: %s", strings.Join(parts, ", "))
}
