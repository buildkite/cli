package preflight

import (
	"net/url"
	"testing"
)

func TestBuildRunSummaryPath(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		query url.Values
		want  string
	}{
		"without query": {
			query: nil,
			want:  "v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1",
		},
		"with query": {
			query: url.Values{
				"failed_result": {"^failed"},
				"include":       {"latest_fail"},
			},
			want: "v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1?failed_result=%5Efailed&include=latest_fail",
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := buildRunSummaryPath("test-org", "build-id-123", tc.query)
			if got != tc.want {
				t.Fatalf("buildRunSummaryPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunSummaryResponse_SummaryResult_PreservesRunsByRunID(t *testing.T) {
	t.Parallel()

	result := RunSummaryResponse{
		Tests: RunSummaryTests{
			Runs: map[string]RunSummaryRun{
				"run-1": {Suite: RunSummarySuite{Slug: "rspec"}, Passed: 10, Failed: 1, Skipped: 2},
				"run-2": {Suite: RunSummarySuite{Slug: "rspec"}, Passed: 12, Failed: 0, Skipped: 1},
			},
			Failures: []RunSummaryFailure{{
				RunID:         "run-1",
				SuiteSlug:     "rspec",
				Name:          "example spec",
				FailureReason: "boom",
			}},
		},
	}.SummaryResult()

	if len(result.Tests) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(result.Tests))
	}

	run1, ok := result.Tests["run-1"]
	if !ok {
		t.Fatal("expected run-1 summary")
	}
	if run1.RunID != "run-1" || run1.SuiteSlug != "rspec" || run1.Passed != 10 || run1.Failed != 1 || run1.Skipped != 2 {
		t.Fatalf("unexpected run-1 summary: %+v", run1)
	}

	run2, ok := result.Tests["run-2"]
	if !ok {
		t.Fatal("expected run-2 summary")
	}
	if run2.RunID != "run-2" || run2.SuiteSlug != "rspec" || run2.Passed != 12 || run2.Failed != 0 || run2.Skipped != 1 {
		t.Fatalf("unexpected run-2 summary: %+v", run2)
	}

	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	if result.Failures[0].RunID != "run-1" {
		t.Fatalf("expected failure run_id to be preserved, got %+v", result.Failures[0])
	}
}
