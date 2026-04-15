package preflight

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
)

func TestShowCmd_Run(t *testing.T) {
	t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")

	preflightID := "00000000-0000-0000-0000-000000000123"

	type requestExpectations struct {
		includeLatestFail bool
	}

	run := func(t *testing.T, cmd ShowCmd, expect requestExpectations) internalpreflight.ShowResult {
		t.Helper()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				if got, want := r.URL.Query()["branch[]"], []string{"bk/preflight/" + preflightID}; len(got) != 1 || got[0] != want[0] {
					t.Errorf("branch filter = %v, want %v", got, want)
				}
				_, _ = w.Write([]byte(`[
				{
					"id": "build-123",
					"number": 123,
					"pipeline": {"slug": "test-pipeline"}
				}
			]`))
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/123":
				if r.URL.Query().Get("include_test_engine") != "true" {
					t.Errorf("include_test_engine query = %q, want true", r.URL.Query().Get("include_test_engine"))
				}
				if r.URL.Query().Get("include_retried_jobs") != "true" {
					t.Errorf("include_retried_jobs query = %q, want true", r.URL.Query().Get("include_retried_jobs"))
				}
				_, _ = w.Write([]byte(`{
				"id": "build-123",
				"number": 123,
				"state": "failed",
				"web_url": "https://buildkite.com/test-org/test-pipeline/builds/123",
				"started_at": "2026-04-10T10:00:00Z",
				"finished_at": "2026-04-10T10:00:23.4Z",
				"test_engine": {
					"runs": [
						{
							"id": "run-1",
							"suite": {"id": "suite-1", "slug": "rspec"}
						}
					]
				},
				"jobs": [
					{
						"id": "job-passed",
						"type": "script",
						"name": "RSpec shard 1",
						"command": "bundle exec rspec spec/models",
						"state": "passed",
						"exit_status": 0,
						"soft_failed": false,
						"retried": false,
						"started_at": "2026-04-10T10:00:01Z"
					},
					{
						"id": "job-failed",
						"type": "script",
						"name": "RSpec shard 2",
						"command": "bundle exec rspec spec/services",
						"state": "failed",
						"exit_status": 1,
						"soft_failed": false,
						"retried": true,
						"started_at": "2026-04-10T10:00:02Z"
					},
					{
						"id": "job-soft-failed",
						"type": "script",
						"name": "Optional checks",
						"command": "./optional-checks",
						"state": "failed",
						"exit_status": 1,
						"soft_failed": true,
						"retried": false,
						"started_at": "2026-04-10T10:00:03Z"
					},
					{
						"id": "wait-1",
						"type": "waiter",
						"state": "passed"
					}
				]
			}`))
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/preflight/runs/build-123":
				if r.URL.Query().Get("build_id") != "build-123" {
					t.Errorf("build_id query = %q, want build-123", r.URL.Query().Get("build_id"))
				}
				if r.URL.Query().Get("failed_result") != "failed" {
					t.Errorf("failed_result query = %q, want failed", r.URL.Query().Get("failed_result"))
				}
				if gotInclude := r.URL.Query().Get("include"); expect.includeLatestFail {
					if gotInclude != "latest_fail" {
						t.Errorf("include query = %q, want latest_fail", gotInclude)
					}
				} else if gotInclude != "" {
					t.Errorf("include query = %q, want empty", gotInclude)
				}
				_, _ = w.Write([]byte(`{
				"tests": {
					"runs": {
						"run-1": {
							"suite": {"id": "suite-1", "slug": "rspec"},
							"passed": 47,
							"failed": 1,
							"skipped": 12
						}
					},
					"failures": [
						{
							"run_id": "run-1",
							"suite_slug": "rspec",
							"name": "AuthService.validateToken handles expired tokens",
							"location": "src/auth.test.ts:89",
							"latest_fail": {
								"failure_reason": "Expected 'expired' but got 'invalid'",
								"failure_expanded": [
									{
										"backtrace": ["", "        src/auth.test.ts:89", "      "],
										"expanded": ["Do not use Array index in keys", "react/no-array-index-key"]
									}
								]
							}
						}
					]
				}
			}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, ".bk.yaml"), []byte("selected_org: test-org\n"), 0o644); err != nil {
			t.Fatalf("writing local config: %v", err)
		}

		stdout := captureStdout(t, func() {
			if err := cmd.Run(nil, stubGlobals{}); err != nil {
				t.Fatalf("ShowCmd.Run returned error: %v", err)
			}
		})

		var got internalpreflight.ShowResult
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("decoding output JSON: %v\noutput=%s", err, stdout)
		}

		return got
	}

	t.Run("without failures flag", func(t *testing.T) {
		got := run(t, ShowCmd{PreflightID: preflightID}, requestExpectations{})

		if got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		if got.DurationMS != 23400 {
			t.Fatalf("duration_ms = %d, want 23400", got.DurationMS)
		}
		if got.BuildURL != "https://buildkite.com/test-org/test-pipeline/builds/123" {
			t.Fatalf("build_url = %q", got.BuildURL)
		}

		suite, ok := got.Tests["rspec"]
		if !ok {
			t.Fatalf("tests missing rspec suite: %#v", got.Tests)
		}
		if suite.Passed != 47 || suite.Failed != 1 || suite.Skipped != 12 {
			t.Fatalf("suite counts = %+v, want passed=47 failed=1 skipped=12", suite)
		}
		if len(suite.Failures) != 0 {
			t.Fatalf("failures len = %d, want 0", len(suite.Failures))
		}

		if got.Jobs.Passed != 1 || got.Jobs.Failed != 1 {
			t.Fatalf("job summary = %+v, want passed=1 failed=1", got.Jobs)
		}
		if len(got.Jobs.FailedJobs) != 1 {
			t.Fatalf("failed_jobs len = %d, want 1", len(got.Jobs.FailedJobs))
		}
		if got.Jobs.FailedJobs[0].ID != "job-failed" || !got.Jobs.FailedJobs[0].Retried {
			t.Fatalf("failed job = %+v", got.Jobs.FailedJobs[0])
		}
	})

	t.Run("with failures flag", func(t *testing.T) {
		got := run(t, ShowCmd{PreflightID: preflightID, Failures: true}, requestExpectations{includeLatestFail: true})

		suite, ok := got.Tests["rspec"]
		if !ok {
			t.Fatalf("tests missing rspec suite: %#v", got.Tests)
		}
		if len(suite.Failures) != 1 {
			t.Fatalf("failures len = %d, want 1", len(suite.Failures))
		}
		if suite.Failures[0].Name != "AuthService.validateToken handles expired tokens" {
			t.Fatalf("failure name = %q", suite.Failures[0].Name)
		}
		if suite.Failures[0].Message != "Expected 'expired' but got 'invalid'" {
			t.Fatalf("failure message = %q", suite.Failures[0].Message)
		}
		if len(suite.Failures[0].FailureDetail) != 1 || len(suite.Failures[0].FailureDetail[0].Expanded) != 2 {
			t.Fatalf("failure detail = %+v", suite.Failures[0].FailureDetail)
		}
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing stdout writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("closing stdout reader: %v", err)
	}

	return string(data)
}
