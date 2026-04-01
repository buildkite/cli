package preflight

import (
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestJobPresenter_FailedLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 183663,
	}.Line(scriptJob("failed-windows-smoke-tests", "Windows smoke tests", "failed", false, &startedAt, &finishedAt, &exitStatus))

	assertStringContainsAll(t, line, []string{
		"✗ Windows smoke tests",
		"failed with exit 1",
		"— bk job log -b 183663 -p buildkite/cli failed-windows-smoke-tests",
	})
}

func TestJobPresenter_SoftFailedLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}

	line := jobPresenter{
		pipeline:    "buildkite",
		buildNumber: 183663,
	}.Line(scriptJob("failed-2", "Bundle Audit", "failed", true, &startedAt, &finishedAt, nil))

	assertStringContainsAll(t, line, []string{
		"⚠ Bundle Audit",
		"soft failed",
		"— bk job log -b 183663 -p buildkite failed-2",
	})
}

func TestJobPresenter_FailedNoExit(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.Line(scriptJob("job-1", "Lint", "failed", false, &startedAt, &finishedAt, nil))

	assertStringContainsAll(t, line, []string{
		"✗ Lint",
		"failed",
		"— bk job log -b 42 -p buildkite/cli job-1",
	})
	if strings.Contains(line, "with exit") {
		t.Fatalf("did not expect exit status when nil: %q", line)
	}
}

func assertStringContainsAll(t *testing.T, got string, want []string) {
	t.Helper()

	for _, needle := range want {
		if !strings.Contains(got, needle) {
			t.Fatalf("missing %q in %q", needle, got)
		}
	}
}
