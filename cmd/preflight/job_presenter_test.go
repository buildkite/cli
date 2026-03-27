package preflight

import (
	"regexp"
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

var presenterANSIPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestTTYJobPresenter_RunningLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-2 * time.Minute)}

	line := stripPresenterANSI(ttyJobPresenter{}.Line(scriptJob("job-1", "Lint", "running", false, &startedAt, nil, nil)))

	assertStringContainsAll(t, line, []string{"● Lint", "running"})
}

func TestTTYJobPresenter_FailedLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}
	exitStatus := 14

	line := stripPresenterANSI(ttyJobPresenter{
		pipeline:    "buildkite",
		buildNumber: 183663,
	}.Line(scriptJob("failed-3", "Yarn Audit", "failed", false, &startedAt, &finishedAt, &exitStatus)))

	assertStringContainsAll(t, line, []string{
		"✗ Yarn Audit",
		"failed",
		"exit 14",
		"failed-3",
		"bk job log -b 183663 -p buildkite failed-3",
	})
}

func TestPlainJobPresenter_Line(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}

	line := plainJobPresenter{
		pipeline:    "buildkite",
		buildNumber: 183663,
	}.Line(scriptJob("failed-2", "Bundle Audit", "failed", true, &startedAt, &finishedAt, nil))

	assertStringContainsAll(t, line, []string{
		"⚠ Bundle Audit",
		"soft failed",
		"failed-2",
		"bk job log -b 183663 -p buildkite failed-2",
	})
}

func TestPlainJobPresenter_FinalLine(t *testing.T) {
	startedAt := buildkite.Timestamp{Time: time.Now().Add(-90 * time.Second)}
	finishedAt := buildkite.Timestamp{Time: time.Now().Add(-15 * time.Second)}

	line := plainJobPresenter{
		pipeline:    "buildkite",
		buildNumber: 183663,
		final:       true,
	}.Line(scriptJob("failed-2", "Bundle Audit", "failed", true, &startedAt, &finishedAt, nil))

	assertStringContainsAll(t, line, []string{
		"⚠ Bundle Audit",
		"failed-2",
		"bk job log -b 183663 -p buildkite failed-2",
	})
	if strings.Contains(line, "soft failed") {
		t.Fatalf("did not expect soft failed label in final failure line: %q", line)
	}
}

func stripPresenterANSI(s string) string {
	return presenterANSIPattern.ReplaceAllString(s, "")
}

func assertStringContainsAll(t *testing.T, got string, want []string) {
	t.Helper()

	for _, needle := range want {
		if !strings.Contains(got, needle) {
			t.Fatalf("missing %q in %q", needle, got)
		}
	}
}
