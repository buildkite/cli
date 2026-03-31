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
	lines := captureTTYLines(t, func(r *ttyRenderer) {
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
	lines := captureTTYLines(t, func(r *ttyRenderer) {
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
		}, buildkite.Build{Number: 42, State: "running"})
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Watching build #42…", "jobs:", "3 running")
	assertNoLineContains(t, lines, "Lint")
	assertNoLineContains(t, lines, "Unit Tests")
	assertNoLineContains(t, lines, "Integration Tests")
}

func TestTTYRenderer_RenderStatus_RunningAndFailingJobs(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitOne := 1
	exitFourteen := 14

	lines := captureTTYLines(t, func(r *ttyRenderer) {
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
		}, buildkite.Build{Number: 183663, State: "running"})
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Watching build #183663…", "jobs:", "3 running", "2 failed", "1 soft failed")
	assertLineContains(t, lines, "ECR Vulnerabilities Scan", "failed-1")
	assertLineContains(t, lines, "Bundle Audit", "soft failed", "failed-2")
	assertLineContains(t, lines, "Yarn Audit", "exit 14", "failed-3")
	assertNoLineContains(t, lines, "RSpec 1")
	assertNoLineContains(t, lines, "RSpec 2")
	assertNoLineContains(t, lines, "RSpec 3")
	assertLineOrder(t, lines, "Yarn Audit", "Watching build #183663…")
}

func TestTTYRenderer_RenderStatus_CombinedSummaryLine(t *testing.T) {
	lines := captureTTYLines(t, func(r *ttyRenderer) {
		err := r.renderStatus(watch.BuildStatus{
			Summary: watch.JobSummary{
				Running: 2,
				Passed:  1,
				Waiting: 3,
				Skipped: 1,
			},
		}, buildkite.Build{Number: 2567, State: "running"})
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Watching build #2567…", "jobs: 2 running, 1 passed, 3 waiting, 1 skipped")
}

func TestTTYRenderer_RenderStatus_KeepsFailuresAboveWatchingLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	lines := captureTTYLines(t, func(r *ttyRenderer) {
		err := r.renderStatus(watch.BuildStatus{
			NewlyFailed: []buildkite.Job{
				scriptJob("failed-1", "Bundle Audit", "failed", false, &startedAt, &finishedAt, &exitStatus),
			},
			Summary: watch.JobSummary{
				Failed: 1,
			},
		}, buildkite.Build{Number: 2567, State: "running"})
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}

		err = r.renderStatus(watch.BuildStatus{
			Summary: watch.JobSummary{
				Running: 2,
				Failed:  1,
			},
		}, buildkite.Build{Number: 2567, State: "running"})
		if err != nil {
			t.Fatalf("renderStatus returned error: %v", err)
		}
	})

	assertLineContains(t, lines, "Bundle Audit", "failed-1")
	assertLineContains(t, lines, "Watching build #2567…", "jobs: 2 running, 1 failed")
	assertLineOrder(t, lines, "Bundle Audit", "Watching build #2567…")
}

func TestTTYRenderer_SetCompletedBuild(t *testing.T) {
	lines := captureTTYLines(t, func(r *ttyRenderer) {
		r.setResult("passed")
	})

	assertTail(t, lines, []string{"✅ Preflight passed!"})
}

func captureTTYLines(t *testing.T, fn func(r *ttyRenderer)) []string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "tty-render-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r := newTTYRenderer(f, "buildkite")
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

func assertTail(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) < len(want) {
		t.Fatalf("got %d lines, want at least %d: %v", len(got), len(want), got)
	}

	tail := got[len(got)-len(want):]
	for i := range want {
		if tail[i] != want[i] {
			t.Fatalf("tail[%d] = %q, want %q; full=%v", i, tail[i], want[i], got)
		}
	}
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

func assertNoLineContains(t *testing.T, got []string, part string) {
	t.Helper()

	for _, line := range got {
		if strings.Contains(line, part) {
			t.Fatalf("unexpected line containing %q:\n%s", part, strings.Join(got, "\n"))
		}
	}
}

func assertLineOrder(t *testing.T, got []string, firstPart string, secondPart string) {
	t.Helper()

	firstIndex := -1
	secondIndex := -1
	for i, line := range got {
		if firstIndex == -1 && strings.Contains(line, firstPart) {
			firstIndex = i
		}
		if secondIndex == -1 && strings.Contains(line, secondPart) {
			secondIndex = i
		}
	}

	if firstIndex == -1 || secondIndex == -1 {
		t.Fatalf("could not find lines containing %q and %q:\n%s", firstPart, secondPart, strings.Join(got, "\n"))
	}
	if firstIndex >= secondIndex {
		t.Fatalf("expected line containing %q to appear before %q:\n%s", firstPart, secondPart, strings.Join(got, "\n"))
	}
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
