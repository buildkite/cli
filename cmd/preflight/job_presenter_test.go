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
		"- bk job log -b 183663 -p buildkite/cli failed-windows-smoke-tests",
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
		"- bk job log -b 183663 -p buildkite failed-2",
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
		"- bk job log -b 42 -p buildkite/cli job-1",
	})
	if strings.Contains(line, "with exit") {
		t.Fatalf("did not expect exit status when nil: %q", line)
	}
}

func TestJobPresenter_PassedLine(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.PassedLine(buildkite.Job{ID: "j1", Name: "Lint", Type: "script", State: "passed"})

	assertStringContainsAll(t, line, []string{"✔ Lint"})
}

func TestJobPresenter_PassedLine_WithEmoji(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.PassedLine(buildkite.Job{ID: "j1", Name: ":checkered_flag: Feature flags", Type: "script", State: "passed"})

	if !strings.Contains(line, "✔") {
		t.Fatalf("expected check mark in %q", line)
	}
	if !strings.Contains(line, "Feature flags") {
		t.Fatalf("expected job name in %q", line)
	}
}

func TestJobPresenter_RetryPassedLine(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.RetryPassedLine(buildkite.Job{ID: "retry-1", Name: "Lint", Type: "script", State: "passed", RetriesCount: 1})

	assertStringContainsAll(t, line, []string{"✔ Lint", "passed on retry", "attempt 2"})
}

func TestJobPresenter_RetryPassedLine_MultipleRetries(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.RetryPassedLine(buildkite.Job{ID: "retry-2", Name: "Test", Type: "script", State: "passed", RetriesCount: 2})

	assertStringContainsAll(t, line, []string{"✔ Test", "passed on retry", "attempt 3"})
}

func TestJobPresenter_ColoredRetryPassedLine(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredRetryPassedLine(buildkite.Job{ID: "retry-1", Name: "Lint", Type: "script", State: "passed", RetriesCount: 1})

	assertStringContainsAll(t, line, []string{"✔", "Lint", "passed on retry", "attempt 2"})
}

func TestJobPresenter_ColoredRetryPassedLine_WithEmoji(t *testing.T) {
	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredRetryPassedLine(buildkite.Job{ID: "retry-1", Name: ":docker: Build image", Type: "script", State: "passed", RetriesCount: 1})

	assertStringContainsAll(t, line, []string{"✔", "Build image", "passed on retry"})
}

func TestJobPresenter_ColoredLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredLine(scriptJob("job-1", "Test", "failed", false, &startedAt, &finishedAt, &exitStatus))

	assertStringContainsAll(t, line, []string{"✗", "Test", "failed with exit 1"})
}

func TestJobPresenter_ColoredLine_UsesClickableJobLink(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	job := scriptJob("job-1", "Test", "failed", false, &startedAt, &finishedAt, &exitStatus)
	job.WebURL = "https://buildkite.com/buildkite/cli/builds/42#job-1"

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredLine(job)

	assertStringContainsAll(t, line, []string{"✗", "Test", "failed with exit 1 - ", "\033[4:4mView job\033[24m", job.WebURL})
	if strings.Contains(line, "bk job log") {
		t.Fatalf("expected clickable job link instead of job log command: %q", line)
	}
}

func TestJobPresenter_ColoredLine_DerivesClickableJobLinkFromBuildURL(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
		buildURL:    "https://buildkite.com/buildkite/cli/builds/42",
	}.ColoredLine(scriptJob("job-1", "Test", "failed", false, &startedAt, &finishedAt, &exitStatus))

	assertStringContainsAll(t, line, []string{"View job", "https://buildkite.com/buildkite/cli/builds/42#job-1"})
	if strings.Contains(line, "bk job log") {
		t.Fatalf("expected derived clickable job link instead of job log command: %q", line)
	}
}

func TestJobPresenter_ColoredLine_SoftFailed(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredLine(scriptJob("job-1", "Audit", "failed", true, &startedAt, &finishedAt, nil))

	assertStringContainsAll(t, line, []string{"⚠", "Audit", "soft failed"})
}

func TestJobPresenter_ColoredLine_WithEmoji(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 1

	line := jobPresenter{
		pipeline:    "buildkite/cli",
		buildNumber: 42,
	}.ColoredLine(scriptJob("job-1", ":docker: Build image", "failed", false, &startedAt, &finishedAt, &exitStatus))

	assertStringContainsAll(t, line, []string{"✗", "Build image", "failed with exit 1"})
}

func assertStringContainsAll(t *testing.T, got string, want []string) {
	t.Helper()

	for _, needle := range want {
		if !strings.Contains(got, needle) {
			t.Fatalf("missing %q in %q", needle, got)
		}
	}
}
