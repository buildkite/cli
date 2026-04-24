package preflight

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/config"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"

	"github.com/buildkite/cli/v3/internal/build/watch"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type stubGlobals struct{}

func (s stubGlobals) SkipConfirmation() bool { return false }
func (s stubGlobals) DisableInput() bool     { return false }
func (s stubGlobals) IsQuiet() bool          { return false }
func (s stubGlobals) DisablePager() bool     { return false }
func (s stubGlobals) EnableDebug() bool      { return false }

var _ cli.GlobalFlags = stubGlobals{}

func TestParseExitConditions(t *testing.T) {
	tests := []struct {
		name        string
		cmd         RunCmd
		wantPolicy  internalpreflight.ExitPolicy
		wantErrText string
	}{
		{name: "defaults to build-failing", cmd: RunCmd{Watch: true, Interval: 1}, wantPolicy: internalpreflight.ExitOnBuildFailing},
		{name: "accepts build-failing", cmd: RunCmd{Watch: true, Interval: 1, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildFailing}}, wantPolicy: internalpreflight.ExitOnBuildFailing},
		{name: "accepts build-terminal", cmd: RunCmd{Watch: true, Interval: 1, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildTerminal}}, wantPolicy: internalpreflight.ExitOnBuildTerminal},
		{name: "accepts repeated build-terminal", cmd: RunCmd{Watch: true, Interval: 1, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildTerminal, internalpreflight.ExitOnBuildTerminal}}, wantPolicy: internalpreflight.ExitOnBuildTerminal},
		{name: "rejects mixed lifecycle policies", cmd: RunCmd{Watch: true, Interval: 1, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildFailing, internalpreflight.ExitOnBuildTerminal}}, wantErrText: "build-failing and build-terminal cannot be used together"},
		{name: "rejects exit-on when watch disabled", cmd: RunCmd{Watch: false, Interval: 1, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildFailing}}, wantErrText: "--exit-on requires --watch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, bkErrors.ErrValidation) {
					t.Fatalf("expected validation error, got %T: %v", err, err)
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrText, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			got := internalpreflight.EffectiveExitPolicy(tt.cmd.ExitOn)
			if got != tt.wantPolicy {
				t.Fatalf("policy = %v, want %v", got, tt.wantPolicy)
			}
		})
	}
}

func TestPreflightCmd_Run(t *testing.T) {
	t.Run("returns validation error when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")

		cmd := &RunCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) {
			t.Fatalf("expected bkErrors.Error, got %T: %v", err, err)
		}
		if !errors.Is(bkErr, bkErrors.ErrValidation) {
			t.Errorf("expected ErrValidation, got category: %v", bkErr.Category)
		}
	})

	t.Run("returns validation error when not in git repo", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		// Run from a temp dir that is not a git repo.
		t.Chdir(t.TempDir())

		cmd := &RunCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) {
			t.Fatalf("expected bkErrors.Error, got %T: %v", err, err)
		}
		if !errors.Is(bkErr, bkErrors.ErrValidation) {
			t.Errorf("expected ErrValidation, got category: %v", bkErr.Category)
		}
	})

	t.Run("build-failing early exit enriches summary with test results", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var buildCancelRequests atomic.Int32
		var buildPolls atomic.Int32
		var summaryRequests atomic.Int32
		var includeLatestFail atomic.Bool
		var stateEnabled atomic.Bool
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/builds/1/cancel"):
				buildCancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "canceling"})
				return

			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				poll := buildPolls.Add(1)
				exitOne := 1
				build := buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "running",
					Jobs: []buildkite.Job{{
						ID:    "job-running",
						Type:  "script",
						Name:  "Lint",
						State: "running",
					}},
				}
				if poll >= 2 {
					build.State = "failing"
					build.TestEngine = &buildkite.TestEngineProperty{
						Runs: []buildkite.TestEngineRun{{
							ID: "run-1",
							Suite: buildkite.TestEngineSuite{
								Slug: "rspec",
							},
						}},
					}
					build.Jobs = []buildkite.Job{{
						ID:         "job-failed",
						Type:       "script",
						Name:       "Lint",
						State:      "failed",
						ExitStatus: &exitOne,
					}}
				}
				json.NewEncoder(w).Encode(build)
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				if r.URL.Query().Get("include") == "latest_fail" {
					includeLatestFail.Store(true)
				}
				if r.URL.Query().Get("state") == "enabled" {
					stateEnabled.Store(true)
				}
				_, _ = w.Write([]byte(`{
					"tests": {
						"runs": {
							"run-1": {
								"suite": {"id": "suite-1", "slug": "rspec", "name": "RSpec"},
								"passed": 47,
								"failed": 1,
								"skipped": 12
							}
						},
						"failures": [
							{
								"run_id": "run-1",
								"suite_name": "RSpec",
								"suite_slug": "rspec",
								"name": "AuthService.validateToken handles expired tokens",
								"location": "src/auth.test.ts:89",
								"latest_fail": {
									"failure_reason": "Expected 'expired' but got 'invalid'"
								}
							}
						]
					}
				}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, JSON: true}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightIncompleteFailure) {
				t.Fatalf("expected incomplete failure error, got %v", err)
			}
		})

		events := decodeJSONLEvents(t, stdout)
		var buildStatusCount int
		var summaries []Event
		for _, event := range events {
			if event.Type == EventBuildStatus {
				buildStatusCount++
			}
			if event.Type == EventBuildSummary {
				summaries = append(summaries, event)
			}
		}

		if buildStatusCount != 2 {
			t.Fatalf("expected 2 build status events before early stop, got %d", buildStatusCount)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected exactly 1 build summary event, got %d", len(summaries))
		}
		summary := summaries[0]
		if !summary.Incomplete {
			t.Fatal("expected summary to be marked incomplete")
		}
		if summary.StopReason != "build-failing" {
			t.Fatalf("expected stop reason build-failing, got %q", summary.StopReason)
		}
		if summary.BuildCanceled == nil || !*summary.BuildCanceled {
			t.Fatalf("expected build_canceled=true, got %#v", summary.BuildCanceled)
		}
		if summary.BuildState != "failing" {
			t.Fatalf("expected failing build state, got %q", summary.BuildState)
		}
		if len(summary.FailedJobs) != 1 || summary.FailedJobs[0].Name != "Lint" {
			t.Fatalf("expected failed jobs in summary, got %#v", summary.FailedJobs)
		}
		if got := summary.Tests.Runs["run-1"]; got.SuiteName != "RSpec" || got.Failed != 1 || got.Passed != 47 || got.Skipped != 12 {
			t.Fatalf("expected enriched test run summary, got %#v", got)
		}
		if len(summary.Tests.Failures) != 1 || summary.Tests.Failures[0].Name != "AuthService.validateToken handles expired tokens" {
			t.Fatalf("expected enriched test failures, got %#v", summary.Tests.Failures)
		}
		if !includeLatestFail.Load() {
			t.Fatal("expected early-exit summary to request latest_fail details")
		}
		if !stateEnabled.Load() {
			t.Fatal("expected early-exit summary to request state=enabled")
		}
		if summaryRequests.Load() != 1 {
			t.Fatalf("expected one preflight summary request, got %d", summaryRequests.Load())
		}
		if buildCancelRequests.Load() != 1 {
			t.Fatalf("expected one build cancel request, got %d", buildCancelRequests.Load())
		}
		if buildPolls.Load() != 3 {
			t.Fatalf("expected three build polls including final summary fetch, got %d", buildPolls.Load())
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/") {
			t.Errorf("expected preflight branch to be cleaned up, but found: %s", refs)
		}
	})

	t.Run("snapshots and creates build", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var gotReq buildkite.CreateBuild
		var gotUserAgent string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				gotUserAgent = r.Header.Get("User-Agent")
				json.NewDecoder(r.Body).Decode(&gotReq)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:      "build-id-123",
					Number:  1,
					State:   "scheduled",
					WebURL:  "https://buildkite.com/test-org/test-pipeline/builds/1",
					Message: gotReq.Message,
					Commit:  gotReq.Commit,
					Branch:  gotReq.Branch,
					URL:     "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/1",
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		// Create a dirty file so the snapshot has something to commit.
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		expectedSourceBranch := runGit(t, worktree, "branch", "--show-current")
		expectedSourceCommit := runGit(t, worktree, "rev-parse", "HEAD")

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: false, Interval: 2}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if gotReq.Commit == "" {
			t.Fatal("expected build creation request with a commit, got empty")
		}
		if !strings.HasPrefix(gotReq.Branch, "bk/preflight/") {
			t.Errorf("expected branch starting with bk/preflight/, got %q", gotReq.Branch)
		}
		if !strings.HasPrefix(gotReq.Message, "Preflight ") {
			t.Errorf("expected message starting with 'Preflight ', got %q", gotReq.Message)
		}
		if gotReq.Env["PREFLIGHT"] != "true" {
			t.Errorf("expected PREFLIGHT=true, got %#v", gotReq.Env)
		}
		if gotReq.Env["BUILDKITE_PREFLIGHT"] != "true" {
			t.Errorf("expected BUILDKITE_PREFLIGHT=true (deprecated), got %#v", gotReq.Env)
		}
		if gotReq.Env["PREFLIGHT_SOURCE_BRANCH"] != expectedSourceBranch {
			t.Errorf("expected PREFLIGHT_SOURCE_BRANCH=%q, got %#v", expectedSourceBranch, gotReq.Env)
		}
		if gotReq.Env["PREFLIGHT_SOURCE_COMMIT"] != expectedSourceCommit {
			t.Errorf("expected PREFLIGHT_SOURCE_COMMIT=%q, got %#v", expectedSourceCommit, gotReq.Env)
		}
		if !strings.Contains(gotUserAgent, buildkite.DefaultUserAgent) {
			t.Errorf("expected User-Agent to contain %q, got %q", buildkite.DefaultUserAgent, gotUserAgent)
		}
		if !strings.Contains(gotUserAgent, "buildkite-cli-preflight/") {
			t.Errorf("expected User-Agent to contain preflight token, got %q", gotUserAgent)
		}
	})

	t.Run("omits source branch env when git HEAD is detached", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var gotReq buildkite.CreateBuild
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				json.NewDecoder(r.Body).Decode(&gotReq)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		expectedSourceCommit := runGit(t, worktree, "rev-parse", "HEAD")
		runGit(t, worktree, "checkout", expectedSourceCommit)

		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: false, Interval: 2}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if _, ok := gotReq.Env["PREFLIGHT_SOURCE_BRANCH"]; ok {
			t.Errorf("expected PREFLIGHT_SOURCE_BRANCH to be omitted in detached HEAD, got %#v", gotReq.Env)
		}
		if gotReq.Env["PREFLIGHT_SOURCE_COMMIT"] != expectedSourceCommit {
			t.Errorf("expected PREFLIGHT_SOURCE_COMMIT=%q, got %#v", expectedSourceCommit, gotReq.Env)
		}
	})

	t.Run("falls back to git cli when factory cannot open repository", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		originalNewFactory := newFactory
		t.Cleanup(func() { newFactory = originalNewFactory })

		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == "POST" && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			case r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1"):
				json.NewEncoder(w).Encode(buildkite.Build{
					Number:     1,
					State:      "passed",
					FinishedAt: &buildkite.Timestamp{Time: now},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()

		newFactory = func(...factory.FactoryOpt) (*factory.Factory, error) {
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
			if err != nil {
				return nil, err
			}
			return &factory.Factory{
				Config:        config.New(nil, nil),
				RestAPIClient: client,
			}, nil
		}

		worktree := initTestRepo(t)
		subdir := filepath.Join(worktree, "nested", "dir")
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(subdir)
		if err := os.WriteFile(filepath.Join(subdir, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
		if err := cmd.Run(nil, stubGlobals{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/") {
			t.Errorf("expected preflight branch to be cleaned up, but found: %s", refs)
		}
	})

	t.Run("watches build until completion and cleans up remote branch", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		pollCount := 0
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1") {
				pollCount++
				b := buildkite.Build{Number: 1, State: "running"}
				if pollCount >= 3 {
					b.State = "passed"
					b.FinishedAt = &buildkite.Timestamp{Time: now}
				}
				json.NewEncoder(w).Encode(b)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if pollCount < 3 {
			t.Errorf("expected at least 3 polls, got %d", pollCount)
		}

		// Verify the remote preflight branch was deleted.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/") {
			t.Errorf("expected preflight branch to be cleaned up, but found: %s", refs)
		}
	})

	t.Run("early exit summary tolerates summary endpoint failure", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var buildCancelRequests atomic.Int32
		var buildPolls atomic.Int32
		var summaryRequests atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/builds/1/cancel"):
				buildCancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "canceling"})
				return

			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				poll := buildPolls.Add(1)
				build := buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "running",
					Jobs: []buildkite.Job{{
						ID:    "job-running",
						Type:  "script",
						Name:  "Lint",
						State: "running",
					}},
				}
				if poll >= 2 {
					exitOne := 1
					build.State = "failing"
					build.Jobs = []buildkite.Job{{
						ID:         "job-failed",
						Type:       "script",
						Name:       "Lint",
						State:      "failed",
						ExitStatus: &exitOne,
					}}
				}
				json.NewEncoder(w).Encode(build)
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"API::Error::NotFound"}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, JSON: true}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightIncompleteFailure) {
				t.Fatalf("expected incomplete failure error, got %v", err)
			}
		})

		events := decodeJSONLEvents(t, stdout)
		var buildStatusCount int
		var summaries []Event
		for _, event := range events {
			if event.Type == EventBuildStatus {
				buildStatusCount++
			}
			if event.Type == EventBuildSummary {
				summaries = append(summaries, event)
			}
		}

		if buildStatusCount != 2 {
			t.Fatalf("expected 2 build status events before early stop, got %d", buildStatusCount)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected exactly 1 build summary event, got %d", len(summaries))
		}
		summary := summaries[0]
		if !summary.Incomplete {
			t.Fatal("expected summary to be marked incomplete")
		}
		if summary.StopReason != "build-failing" {
			t.Fatalf("expected stop reason build-failing, got %q", summary.StopReason)
		}
		if summary.BuildCanceled == nil || !*summary.BuildCanceled {
			t.Fatalf("expected build_canceled=true, got %#v", summary.BuildCanceled)
		}
		if summary.BuildState != "failing" {
			t.Fatalf("expected failing build state, got %q", summary.BuildState)
		}
		if len(summary.FailedJobs) != 1 || summary.FailedJobs[0].Name != "Lint" {
			t.Fatalf("expected failed jobs in summary, got %#v", summary.FailedJobs)
		}
		if len(summary.Tests.Runs) != 0 || len(summary.Tests.Failures) != 0 {
			t.Fatalf("expected no enriched tests when summary endpoint fails, got %#v", summary.Tests)
		}
		if summaryRequests.Load() != 1 {
			t.Fatalf("expected one preflight summary request, got %d", summaryRequests.Load())
		}
		if buildCancelRequests.Load() != 1 {
			t.Fatalf("expected one build cancel request, got %d", buildCancelRequests.Load())
		}
		if buildPolls.Load() != 3 {
			t.Fatalf("expected three build polls including final summary fetch, got %d", buildPolls.Load())
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/") {
			t.Errorf("expected preflight branch to be cleaned up, but found: %s", refs)
		}
	})

	t.Run("no-cleanup leaves branch and build running after early stop", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var buildCancelRequests atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/builds/1/cancel"):
				buildCancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "canceling"})
				return

			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				exitOne := 1
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "failing",
					Jobs: []buildkite.Job{{
						ID:         "job-failed",
						Type:       "script",
						Name:       "Lint",
						State:      "failed",
						ExitStatus: &exitOne,
					}},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, JSON: true, NoCleanup: true}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightIncompleteFailure) {
				t.Fatalf("expected incomplete failure error, got %v", err)
			}
		})

		events := decodeJSONLEvents(t, stdout)
		var summaries []Event
		for _, event := range events {
			if event.Type == EventBuildSummary {
				summaries = append(summaries, event)
			}
		}

		if len(summaries) != 1 {
			t.Fatalf("expected exactly 1 build summary event, got %d", len(summaries))
		}
		summary := summaries[0]
		if summary.BuildCanceled == nil || *summary.BuildCanceled {
			t.Fatalf("expected build_canceled=false, got %#v", summary.BuildCanceled)
		}
		if buildCancelRequests.Load() != 0 {
			t.Fatalf("expected no build cancel requests, got %d", buildCancelRequests.Load())
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/") {
			t.Error("expected preflight branch to still exist with --no-cleanup")
		}
	})

	t.Run("build-terminal waits for terminal completion", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var buildCancelRequests atomic.Int32
		var buildPolls atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/builds/1/cancel"):
				buildCancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "canceling"})
				return

			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				poll := buildPolls.Add(1)
				exitOne := 1
				build := buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "running",
					Jobs: []buildkite.Job{{
						ID:    "job-running",
						Type:  "script",
						Name:  "Lint",
						State: "running",
					}},
				}
				if poll >= 2 {
					build.State = "failing"
					build.Jobs = []buildkite.Job{{
						ID:         "job-failed",
						Type:       "script",
						Name:       "Lint",
						State:      "failed",
						ExitStatus: &exitOne,
					}}
				}
				if poll >= 3 {
					build.State = "failed"
					build.FinishedAt = &buildkite.Timestamp{Time: now}
				}
				json.NewEncoder(w).Encode(build)
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, JSON: true, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildTerminal}}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
				t.Fatalf("expected completed failure error, got %v", err)
			}
		})

		events := decodeJSONLEvents(t, stdout)
		var buildStatusCount int
		var summaries []Event
		for _, event := range events {
			if event.Type == EventBuildStatus {
				buildStatusCount++
			}
			if event.Type == EventBuildSummary {
				summaries = append(summaries, event)
			}
		}

		if buildStatusCount != 3 {
			t.Fatalf("expected 3 build status events before terminal exit, got %d", buildStatusCount)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected exactly 1 build summary event, got %d", len(summaries))
		}
		summary := summaries[0]
		if summary.Incomplete {
			t.Fatal("expected terminal summary, got incomplete=true")
		}
		if summary.StopReason != "" {
			t.Fatalf("expected empty stop reason, got %q", summary.StopReason)
		}
		if summary.BuildCanceled != nil {
			t.Fatalf("expected no build_canceled metadata for terminal summary, got %#v", summary.BuildCanceled)
		}
		if summary.BuildState != "failed" {
			t.Fatalf("expected failed build state, got %q", summary.BuildState)
		}
		if buildCancelRequests.Load() != 0 {
			t.Fatalf("expected no build cancel requests, got %d", buildCancelRequests.Load())
		}
		if buildPolls.Load() < 3 {
			t.Fatalf("expected to keep polling through terminal state, got %d polls", buildPolls.Load())
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/") {
			t.Errorf("expected preflight branch to be cleaned up, but found: %s", refs)
		}
	})

	t.Run("final summary does not retry test results without await flag", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var includeLatestFail atomic.Bool
		var summaryRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     1,
					State:      "failed",
					WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/1",
					FinishedAt: &buildkite.Timestamp{Time: now},
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
					TestEngine: &buildkite.TestEngineProperty{
						Runs: []buildkite.TestEngineRun{{
							ID: "run-1",
							Suite: buildkite.TestEngineSuite{
								Slug: "rspec",
							},
						}},
					},
					Jobs: []buildkite.Job{{
						ID:    "job-failed",
						Type:  "script",
						Name:  "RSpec shard 1",
						State: "failed",
					}},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 1,
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				}})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				if r.URL.Query().Get("include") == "latest_fail" {
					includeLatestFail.Store(true)
				}
				if summaryRequests.Load() == 1 {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"message":"API::Error::NotFound"}`))
					return
				}
				_, _ = w.Write([]byte(`{
					"tests": {
						"runs": {
							"run-1": {
								"suite": {"id": "suite-1", "slug": "rspec", "name": "RSpec"},
								"passed": 47,
								"failed": 1,
								"skipped": 12
							}
						},
						"failures": [
							{
								"run_id": "run-1",
								"suite_name": "RSpec",
								"suite_slug": "rspec",
								"name": "AuthService.validateToken handles expired tokens",
								"location": "src/auth.test.ts:89",
								"latest_fail": {
									"failure_reason": "Expected 'expired' but got 'invalid'"
								}
							}
						]
					}
				}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, Text: true}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
				t.Fatalf("expected completed failure error, got %v", err)
			}
		})

		if !includeLatestFail.Load() {
			t.Fatal("expected preflight summary to request latest_fail details")
		}
		if got := summaryRequests.Load(); got != 1 {
			t.Fatalf("expected one summary request without await flag, got %d", got)
		}
		if strings.Contains(stdout, "AuthService.validateToken handles expired tokens") {
			t.Fatalf("expected no endpoint failure name in final summary, got %q", stdout)
		}
		if strings.Contains(stdout, "Expected 'expired' but got 'invalid'") {
			t.Fatalf("expected no endpoint failure message in final summary, got %q", stdout)
		}
	})

	t.Run("final summary tolerates transient build lookup failure without await flag", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var buildRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				if buildRequests.Add(1) == 1 {
					json.NewEncoder(w).Encode(buildkite.Build{
						ID:         "build-id-123",
						Number:     1,
						State:      "failed",
						WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/1",
						FinishedAt: &buildkite.Timestamp{Time: now},
						Pipeline: &buildkite.Pipeline{
							Slug: "test-pipeline",
						},
						Jobs: []buildkite.Job{{
							ID:    "job-failed",
							Type:  "script",
							Name:  "RSpec shard 1",
							State: "failed",
						}},
					})
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"temporary failure"}`))
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 1,
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				}})
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, Text: true}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
				t.Fatalf("expected completed failure error, got %v", err)
			}
		})

		if !strings.Contains(stdout, "❌ Preflight Failed") {
			t.Fatalf("expected final summary header, got %q", stdout)
		}
		if !strings.Contains(stdout, "RSpec shard 1") {
			t.Fatalf("expected failed job in final summary, got %q", stdout)
		}
	})

	t.Run("await-test-results loads summary after timeout", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var includeLatestFail atomic.Bool
		var stateEnabled atomic.Bool
		var summaryRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     1,
					State:      "failed",
					WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/1",
					FinishedAt: &buildkite.Timestamp{Time: now},
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
					TestEngine: &buildkite.TestEngineProperty{
						Runs: []buildkite.TestEngineRun{{
							ID: "run-1",
							Suite: buildkite.TestEngineSuite{
								Slug: "rspec",
							},
						}},
					},
					Jobs: []buildkite.Job{{
						ID:    "job-failed",
						Type:  "script",
						Name:  "RSpec shard 1",
						State: "failed",
					}},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 1,
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				}})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				if r.URL.Query().Get("include") == "latest_fail" {
					includeLatestFail.Store(true)
				}
				if r.URL.Query().Get("state") == "enabled" {
					stateEnabled.Store(true)
				}
				_, _ = w.Write([]byte(`{
						"tests": {
						"runs": {
							"run-1": {
								"suite": {"id": "suite-1", "slug": "rspec", "name": "RSpec"},
								"passed": 47,
								"failed": 1,
								"skipped": 12
							}
						},
						"failures": [
							{
								"run_id": "run-1",
								"suite_name": "RSpec",
								"suite_slug": "rspec",
								"name": "AuthService.validateToken handles expired tokens",
								"location": "src/auth.test.ts:89",
								"latest_fail": {
									"failure_reason": "Expected 'expired' but got 'invalid'"
								}
							}
						]
					}
				}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{
				Pipeline:         "test-org/test-pipeline",
				Watch:            true,
				Interval:         0.01,
				Text:             true,
				AwaitTestResults: awaitTestResultsFlag{Enabled: true, Duration: 35 * time.Millisecond},
			}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
				t.Fatalf("expected completed failure error, got %v", err)
			}
		})

		if !includeLatestFail.Load() {
			t.Fatal("expected preflight summary to request latest_fail details")
		}
		if !stateEnabled.Load() {
			t.Fatal("expected preflight summary to request state=enabled")
		}
		if got := summaryRequests.Load(); got != 1 {
			t.Fatalf("expected one delayed summary request, got %d", got)
		}
		if !strings.Contains(stdout, "✗ RSpec  1 failed  47 passed  12 skipped") {
			t.Fatalf("expected suite name in final summary, got %q", stdout)
		}
		if !strings.Contains(stdout, "✗ [RSpec]") {
			t.Fatalf("expected suite name in failure label, got %q", stdout)
		}
		if !strings.Contains(stdout, "AuthService.validateToken handles expired tokens") {
			t.Fatalf("expected endpoint failure name in final summary, got %q", stdout)
		}
		if strings.Contains(stdout, "Expected 'expired' but got 'invalid'") {
			t.Fatalf("expected final summary to omit endpoint failure message, got %q", stdout)
		}
	})

	t.Run("await-test-results timeout still renders final summary", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var summaryRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     1,
					State:      "failed",
					WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/1",
					FinishedAt: &buildkite.Timestamp{Time: now},
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
					TestEngine: &buildkite.TestEngineProperty{
						Runs: []buildkite.TestEngineRun{{
							ID: "run-1",
							Suite: buildkite.TestEngineSuite{
								Slug: "rspec",
							},
						}},
					},
					Jobs: []buildkite.Job{{
						ID:    "job-failed",
						Type:  "script",
						Name:  "RSpec shard 1",
						State: "failed",
					}},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 1,
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				}})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"API::Error::NotFound"}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stdout := captureStdout(t, func() {
			cmd := &RunCmd{
				Pipeline:         "test-org/test-pipeline",
				Watch:            true,
				Interval:         0.01,
				Text:             true,
				AwaitTestResults: awaitTestResultsFlag{Enabled: true, Duration: 35 * time.Millisecond},
			}
			err := cmd.Run(nil, stubGlobals{})
			var bkErr *bkErrors.Error
			if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
				t.Fatalf("expected completed failure error, got %v", err)
			}
		})

		if got := summaryRequests.Load(); got != 1 {
			t.Fatalf("expected one delayed summary request during await timeout, got %d", got)
		}
		if !strings.Contains(stdout, "❌ Preflight Failed") {
			t.Fatalf("expected final summary header, got %q", stdout)
		}
		if strings.Contains(stdout, "AuthService.validateToken handles expired tokens") {
			t.Fatalf("expected no endpoint failure name in final summary, got %q", stdout)
		}
	})

	t.Run("await-test-results does not wait when no test runs are expected", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var summaryRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch {
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/builds/1"):
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     1,
					State:      "failed",
					WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/1",
					FinishedAt: &buildkite.Timestamp{Time: now},
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
					Jobs: []buildkite.Job{{
						ID:    "job-failed",
						Type:  "script",
						Name:  "RSpec shard 1",
						State: "failed",
					}},
				})
				return

			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tests"):
				json.NewEncoder(w).Encode([]buildkite.BuildTest{})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 1,
					Pipeline: &buildkite.Pipeline{
						Slug: "test-pipeline",
					},
				}})
				return

			case r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test-org/builds/build-id-123/preflight/v1":
				summaryRequests.Add(1)
				_, _ = w.Write([]byte(`{"tests":{"runs":{},"failures":[]}}`))
				return
			}

			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{
			Pipeline:         "test-org/test-pipeline",
			Watch:            true,
			Interval:         0.01,
			AwaitTestResults: awaitTestResultsFlag{Enabled: true, Duration: 35 * time.Millisecond},
		}
		err := cmd.Run(nil, stubGlobals{})
		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightCompletedFailure) {
			t.Fatalf("expected completed failure error, got %v", err)
		}
		if got := summaryRequests.Load(); got != 0 {
			t.Fatalf("expected no summary requests when no test runs are expected, got %d", got)
		}
	})

	t.Run("no-cleanup preserves remote branch", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number:     1,
					State:      "passed",
					FinishedAt: &buildkite.Timestamp{Time: now},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, NoCleanup: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Verify the remote preflight branch still exists.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/") {
			t.Error("expected preflight branch to still exist with --no-cleanup")
		}
	})

	t.Run("returns error when build fails", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number:     1,
					State:      "failed",
					FinishedAt: &buildkite.Timestamp{Time: now},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "preflight completed with failure: build is failed") {
			t.Errorf("expected completed failure error, got: %v", err)
		}
	})

	t.Run("returns user aborted error when interrupted while watching", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		originalNotifyContext := notifyContext
		t.Cleanup(func() { notifyContext = originalNotifyContext })

		watchCtx, cancelWatch := context.WithCancel(context.Background())
		notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			return watchCtx, cancelWatch
		}

		var buildCancelRequests atomic.Int32
		var pollCount atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == "PUT" && strings.Contains(r.URL.Path, "/builds/1/cancel"):
				buildCancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "canceling"})
				return
			case r.Method == "POST" && strings.Contains(r.URL.Path, "/builds"):
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			case r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1"):
				if pollCount.Add(1) == 1 {
					cancelWatch()
				}
				json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "running"})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !bkErrors.IsUserAborted(err) {
			t.Fatalf("expected user aborted error, got %T: %v", err, err)
		}
		if code := bkErrors.GetExitCodeForError(err); code != bkErrors.ExitCodeUserAbortedError {
			t.Fatalf("expected exit code %d, got %d", bkErrors.ExitCodeUserAbortedError, code)
		}
		if pollCount.Load() == 0 {
			t.Fatal("expected at least one build poll before interrupt")
		}
		if buildCancelRequests.Load() != 1 {
			t.Fatalf("expected one build cancel request, got %d", buildCancelRequests.Load())
		}
	})

	t.Run("aborts after 10 consecutive polling errors", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		pollCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
				json.NewEncoder(w).Encode(buildkite.Build{
					Number: 1,
					State:  "scheduled",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/1",
				})
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1") {
				pollCount++
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "watching build failed") {
			t.Errorf("expected 'watching build failed', got: %v", err)
		}
		if pollCount < watch.DefaultMaxConsecutiveErrors {
			t.Errorf("expected at least %d polls, got %d", watch.DefaultMaxConsecutiveErrors, pollCount)
		}
	})

	t.Run("returns error when build creation fails", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Pipeline not found"}`))
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Interval: 2}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "creating preflight build") {
			t.Fatalf("expected build creation error, got: %v", err)
		}
	})

	t.Run("closes renderer when build creation fails", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		originalRendererFactory := rendererFactory
		fakeRenderer := &recordingRenderer{}
		rendererFactory = func(io.Writer, bool, bool, context.CancelFunc) renderer {
			return fakeRenderer
		}
		t.Cleanup(func() { rendererFactory = originalRendererFactory })

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Authentication required"}`))
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &RunCmd{Pipeline: "test-org/test-pipeline", Interval: 2}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "creating preflight build") {
			t.Fatalf("expected build creation error, got: %v", err)
		}
		if fakeRenderer.closeCalls != 1 {
			t.Fatalf("expected renderer to be closed once, got %d", fakeRenderer.closeCalls)
		}
	})
}

type recordingRenderer struct {
	closeCalls int
}

func (r *recordingRenderer) Render(Event) error { return nil }

func (r *recordingRenderer) Close() error {
	r.closeCalls++
	return nil
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	worktree := filepath.Join(dir, "work")
	bare := filepath.Join(dir, "origin.git")

	runGit(t, "", "init", "--bare", bare)
	runGit(t, "", "init", worktree)
	runGit(t, worktree, "config", "user.email", "test@test.com")
	runGit(t, worktree, "config", "user.name", "Test")

	initial := filepath.Join(worktree, "README.md")
	if err := os.WriteFile(initial, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "add", ".")
	runGit(t, worktree, "commit", "-m", "initial commit")
	runGit(t, worktree, "remote", "add", "origin", bare)

	return worktree
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
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

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}

	return string(out)
}

func decodeJSONLEvents(t *testing.T, output string) []Event {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	var events []Event
	for {
		var event Event
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return events
			}
			t.Fatalf("decode JSONL event: %v\noutput:\n%s", err, output)
		}
		events = append(events, event)
	}
}
