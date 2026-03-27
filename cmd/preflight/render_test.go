package preflight

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/watch"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

var renderANSIPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestTTYRenderer_SetSnapshot(t *testing.T) {
	lines := captureTTYLines(t, 42, func(r *ttyRenderer) {
		r.setSnapshot(&internalpreflight.SnapshotResult{
			Commit: "1234567890abcdef",
			Ref:    "refs/heads/bk/preflight/abc123",
			Files: []internalpreflight.FileChange{
				{Status: "M", Path: "cmd/preflight/render.go"},
				{Status: "A", Path: "cmd/preflight/render_test.go"},
				{Status: "D", Path: "old/file.txt"},
			},
		})
	})

	assertLineEquals(t, lines, "Commit: 1234567890")
	assertLineEquals(t, lines, "Ref:    refs/heads/bk/preflight/abc123")
	assertLineEquals(t, lines, "Files:  3 changed")
	assertLineEquals(t, lines, "  ~ cmd/preflight/render.go")
	assertLineEquals(t, lines, "  + cmd/preflight/render_test.go")
	assertLineEquals(t, lines, "  - old/file.txt")
}

func TestTTYRenderer_RenderStatus_AllRunningJobs(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-2 * time.Minute)}
	lines := captureTTYLines(t, 42, func(r *ttyRenderer) {
		err := r.renderStatus(watch.BuildStatus{
			Running: []buildkite.Job{
				scriptJob("job-1", "Lint", "running", false, &startedAt, nil, nil),
				scriptJob("job-2", "Unit Tests", "running", false, &startedAt, nil, nil),
				scriptJob("job-3", "Integration Tests", "running", false, &startedAt, nil, nil),
			},
			TotalRunning: 3,
			Summary: watch.JobSummary{
				Running: 3,
			},
		}, "running")
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Watching build #42…")
	assertLineContains(t, lines, "● Lint", "running")
	assertLineContains(t, lines, "● Unit Tests", "running")
	assertLineContains(t, lines, "● Integration Tests", "running")
	assertLineContains(t, lines, "…")
}

func TestTTYRenderer_RenderStatus_RunningAndFailingJobs(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitOne := 1
	exitFourteen := 14

	lines := captureTTYLines(t, 183663, func(r *ttyRenderer) {
		err := r.renderStatus(watch.BuildStatus{
			NewlyFailed: []buildkite.Job{
				scriptJob("failed-1", "ECR Vulnerabilities Scan", "failed", false, &startedAt, &finishedAt, &exitOne),
				scriptJob("failed-2", "Bundle Audit", "failed", true, &startedAt, &finishedAt, &exitOne),
				scriptJob("failed-3", "Yarn Audit", "failed", false, &startedAt, &finishedAt, &exitFourteen),
			},
			Running: []buildkite.Job{
				scriptJob("running-1", "RSpec 1", "running", false, &startedAt, nil, nil),
				scriptJob("running-2", "RSpec 2", "running", false, &startedAt, nil, nil),
				scriptJob("running-3", "RSpec 3", "running", false, &startedAt, nil, nil),
			},
			TotalRunning: 3,
			Summary: watch.JobSummary{
				Failed:     2,
				SoftFailed: 1,
				Running:    3,
			},
		}, "running")
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Watching build #183663…")
	assertLineContains(t, lines, "✗ ECR Vulnerabilities Scan", "failed-1")
	assertLineContains(t, lines, "✗ Bundle Audit", "soft failed", "failed-2")
	assertLineContains(t, lines, "✗ Yarn Audit", "exit 14", "failed-3")
	assertLineContains(t, lines, "● RSpec 1", "running")
	assertLineContains(t, lines, "● RSpec 2", "running")
	assertLineContains(t, lines, "● RSpec 3", "running")
	assertLineContains(t, lines, "2 failed", "1 soft failed")
}
func captureTTYLines(t *testing.T, buildNumber int, fn func(r *ttyRenderer)) []string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "tty-render-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r := newTTYRenderer(f, "buildkite", buildNumber)
	fn(r)

	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	return visibleRenderLines(string(out))
}

func visibleRenderLines(raw string) []string {
	raw = renderANSIPattern.ReplaceAllString(raw, "")
	raw = strings.ReplaceAll(raw, "\r", "")

	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func assertLineEquals(t *testing.T, got []string, want string) {
	t.Helper()

	for _, line := range got {
		if line == want {
			return
		}
	}

	t.Fatalf("missing line %q:\n%s", want, strings.Join(got, "\n"))
}

func assertLineContains(t *testing.T, got []string, parts ...string) {
	t.Helper()

	for _, line := range got {
		matched := true
		for _, part := range parts {
			if !strings.Contains(line, part) {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}

	t.Fatalf("no line contained all parts %v:\n%s", parts, strings.Join(got, "\n"))
}

func scriptJob(id, name, state string, softFailed bool, startedAt, finishedAt *buildkite.Timestamp, exitStatus *int) buildkite.Job {
	return buildkite.Job{
		ID:         id,
		Name:       name,
		Type:       "script",
		State:      state,
		SoftFailed: softFailed,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		ExitStatus: exitStatus,
	}
}
