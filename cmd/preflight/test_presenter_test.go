package preflight

import (
	"strings"
	"testing"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
)

func TestTestPresenter_SummarySuiteLine(t *testing.T) {
	got := testPresenter{}.SummarySuiteLine(
		internalpreflight.SummaryTestRun{SuiteName: "RSpec", Passed: 47, Failed: 2, Skipped: 3},
		summarySuiteColumnWidths{Label: 7, Failed: 1, Passed: 2, Skipped: 1},
	)

	if got != "✗ RSpec    2 failed  47 passed  3 skipped" {
		t.Fatalf("unexpected suite summary line: %q", got)
	}
}

func TestTestPresenter_SummaryFailureLine_WrapsAndIndents(t *testing.T) {
	got := testPresenter{}.SummaryFailureLine(internalpreflight.SummaryTestFailure{
		SuiteName: "RSpec",
		Location:  "src/auth.test.ts:89",
		Name:      "AuthService.validateToken handles expired tokens and reports the reason cleanly",
		Message:   "Expected 'expired' but got 'invalid' while validating the response payload",
	}, 60, "        ")

	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped failure line, got %q", got)
	}

	for _, line := range lines {
		if !strings.HasPrefix(line, "        ") {
			t.Fatalf("expected indented wrapped line, got %q", line)
		}
	}

	if !strings.Contains(got, "✗ [RSpec] src/auth.test.ts:89") {
		t.Fatalf("expected suite-prefixed failure line, got %q", got)
	}
	if strings.Contains(got, "Expected 'expired' but got 'invalid'") {
		t.Fatalf("expected summary failure line to omit failure message, got %q", got)
	}
}
