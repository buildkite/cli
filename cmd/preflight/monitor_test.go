package preflight

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestMonitorCmd_Run(t *testing.T) {
	t.Run("finds a build for the current branch and HEAD commit", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)
		branch := runGit(t, worktree, "branch", "--show-current")
		commit := runGit(t, worktree, "rev-parse", "HEAD")

		var gotBranch string
		var gotCommit string
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds":
				gotBranch = r.URL.Query().Get("branch[]")
				gotCommit = r.URL.Query().Get("commit")
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 42,
					State:  "running",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/42",
				}})
				return
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/42":
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     42,
					State:      "passed",
					FinishedAt: &buildkite.Timestamp{Time: now},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &MonitorCmd{Pipeline: "test-org/test-pipeline", Interval: 0.01, WaitForBuild: time.Second, Text: true}
		if err := cmd.Run(nil, stubGlobals{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if gotBranch != branch {
			t.Fatalf("expected branch query %q, got %q", branch, gotBranch)
		}
		if gotCommit != commit {
			t.Fatalf("expected commit query %q, got %q", commit, gotCommit)
		}
	})

	t.Run("waits for a delayed webhook build", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		var listRequests atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds":
				if listRequests.Add(1) == 1 {
					json.NewEncoder(w).Encode([]buildkite.Build{})
					return
				}
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 42,
					State:  "running",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/42",
				}})
				return
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/42":
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:         "build-id-123",
					Number:     42,
					State:      "passed",
					FinishedAt: &buildkite.Timestamp{Time: now},
				})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &MonitorCmd{Pipeline: "test-org/test-pipeline", Interval: 0.01, WaitForBuild: time.Second, Text: true}
		if err := cmd.Run(nil, stubGlobals{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if listRequests.Load() < 2 {
			t.Fatalf("expected at least two build lookup requests, got %d", listRequests.Load())
		}
	})

	t.Run("times out clearly when no build appears", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds" {
				json.NewEncoder(w).Encode([]buildkite.Build{})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &MonitorCmd{Pipeline: "test-org/test-pipeline", Interval: 0.01, WaitForBuild: 20 * time.Millisecond, Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrValidation) {
			t.Fatalf("expected validation error, got %T: %v", err, err)
		}
		if !strings.Contains(bkErr.Details, "no Buildkite build found") {
			t.Fatalf("expected no build details, got %q", bkErr.Details)
		}
		if len(bkErr.Suggestions) == 0 || !strings.Contains(strings.Join(bkErr.Suggestions, "\n"), "Ensure the commit has been pushed") {
			t.Fatalf("expected push suggestion, got %#v", bkErr.Suggestions)
		}
	})

	t.Run("watches a found build until passed", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		var buildPolls atomic.Int32
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 42,
					State:  "running",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/42",
				}})
				return
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/42":
				poll := buildPolls.Add(1)
				build := buildkite.Build{ID: "build-id-123", Number: 42, State: "running"}
				if poll >= 2 {
					build.State = "passed"
					build.FinishedAt = &buildkite.Timestamp{Time: now}
				}
				json.NewEncoder(w).Encode(build)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &MonitorCmd{Pipeline: "test-org/test-pipeline", Interval: 0.01, WaitForBuild: time.Second, Text: true}
		if err := cmd.Run(nil, stubGlobals{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if buildPolls.Load() < 2 {
			t.Fatalf("expected at least two build polls, got %d", buildPolls.Load())
		}
	})

	t.Run("exits early on build failing without canceling the build", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		var cancelRequests atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPut && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/42/cancel":
				cancelRequests.Add(1)
				json.NewEncoder(w).Encode(buildkite.Build{Number: 42, State: "canceling"})
				return
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds":
				json.NewEncoder(w).Encode([]buildkite.Build{{
					ID:     "build-id-123",
					Number: 42,
					State:  "running",
					WebURL: "https://buildkite.com/test-org/test-pipeline/builds/42",
				}})
				return
			case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/pipelines/test-pipeline/builds/42":
				exitOne := 1
				json.NewEncoder(w).Encode(buildkite.Build{
					ID:     "build-id-123",
					Number: 42,
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
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &MonitorCmd{Pipeline: "test-org/test-pipeline", Interval: 0.01, WaitForBuild: time.Second, Text: true}
		err := cmd.Run(nil, stubGlobals{})
		var bkErr *bkErrors.Error
		if !errors.As(err, &bkErr) || !errors.Is(bkErr, bkErrors.ErrPreflightIncompleteFailure) {
			t.Fatalf("expected incomplete failure error, got %T: %v", err, err)
		}
		if cancelRequests.Load() != 0 {
			t.Fatalf("expected no cancel requests, got %d", cancelRequests.Load())
		}
	})
}

func TestMonitorCmd_Validate(t *testing.T) {
	t.Run("accepts exit-on policies", func(t *testing.T) {
		cmd := MonitorCmd{Interval: 1, WaitForBuild: time.Second, ExitOn: []internalpreflight.ExitPolicy{internalpreflight.ExitOnBuildTerminal}}
		if err := cmd.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("rejects invalid interval", func(t *testing.T) {
		cmd := MonitorCmd{Interval: 0, WaitForBuild: time.Second}
		if err := cmd.Validate(); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
