package preflight

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestCleanupCmd_Run(t *testing.T) {
	t.Run("returns validation error when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")

		cmd := &CleanupCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !bkErrors.IsValidationError(err) {
			t.Fatalf("expected validation error, got %T: %v", err, err)
		}
	})

	t.Run("deletes completed branches and skips running ones", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		// Create a test repo with two preflight branches.
		worktree := initTestRepo(t)
		t.Chdir(worktree)

		// Create two preflight branches by pushing commits.
		createPreflightBranch(t, worktree, "bk/preflight/completed-one")
		createPreflightBranch(t, worktree, "bk/preflight/still-running")

		// Verify both branches exist on remote.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/completed-one") {
			t.Fatal("expected completed-one branch to exist")
		}
		if !strings.Contains(refs, "bk/preflight/still-running") {
			t.Fatal("expected still-running branch to exist")
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds") {
				branches := r.URL.Query()["branch[]"]
				var builds []buildkite.Build
				for _, branch := range branches {
					switch branch {
					case "bk/preflight/completed-one":
						builds = append(builds, buildkite.Build{Number: 1, State: "passed", Branch: branch})
					case "bk/preflight/still-running":
						builds = append(builds, buildkite.Build{Number: 2, State: "running", Branch: branch})
					}
				}
				json.NewEncoder(w).Encode(builds)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Verify completed branch was deleted.
		refs = runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/completed-one") {
			t.Error("expected completed-one branch to be deleted")
		}

		// Verify running branch was preserved.
		if !strings.Contains(refs, "bk/preflight/still-running") {
			t.Error("expected still-running branch to be preserved")
		}
	})

	t.Run("reports no branches when none exist", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("deletes orphaned branches with no builds", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		createPreflightBranch(t, worktree, "bk/preflight/orphaned")

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds") {
				json.NewEncoder(w).Encode([]buildkite.Build{})
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/orphaned") {
			t.Error("expected orphaned branch to be deleted")
		}
	})

	t.Run("falls back to git cli when factory has no repository", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		originalNewFactory := newFactory
		t.Cleanup(func() { newFactory = originalNewFactory })

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds") {
				branches := r.URL.Query()["branch[]"]
				var builds []buildkite.Build
				for _, branch := range branches {
					builds = append(builds, buildkite.Build{Number: 1, State: "failed", Branch: branch})
				}
				json.NewEncoder(w).Encode(builds)
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
		t.Chdir(worktree)

		createPreflightBranch(t, worktree, "bk/preflight/to-clean")

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		if err := cmd.Run(nil, stubGlobals{}); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if strings.Contains(refs, "bk/preflight/to-clean") {
			t.Error("expected branch to be deleted")
		}
	})

	t.Run("returns error when API fails", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		createPreflightBranch(t, worktree, "bk/preflight/some-branch")

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds") {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error when API fails, got nil")
		}

		// Branch should still exist since the error prevented cleanup.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/some-branch") {
			t.Error("expected branch to be preserved when API fails")
		}
	})

	t.Run("dry run shows branches without deleting them", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		createPreflightBranch(t, worktree, "bk/preflight/dry-run-test")

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds") {
				branches := r.URL.Query()["branch[]"]
				var builds []buildkite.Build
				for _, branch := range branches {
					builds = append(builds, buildkite.Build{Number: 1, State: "passed", Branch: branch})
				}
				json.NewEncoder(w).Encode(builds)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true, DryRun: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Branch should still exist after dry run.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/dry-run-test") {
			t.Error("expected branch to be preserved during dry run")
		}
	})

	t.Run("stops processing when context is cancelled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		createPreflightBranch(t, worktree, "bk/preflight/cancel-a")
		createPreflightBranch(t, worktree, "bk/preflight/cancel-b")

		// Override notifyContext to return an already-cancelled context so
		// the API call in ResolveBuilds returns context.Canceled immediately.
		originalNotify := notifyContext
		notifyContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			cancel()
			return ctx, cancel
		}
		t.Cleanup(func() { notifyContext = originalNotify })

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer s.Close()
		t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)

		cmd := &CleanupCmd{Pipeline: "test-org/test-pipeline", Text: true}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error on cancellation, got: %v", err)
		}

		// Both branches should still exist since cleanup was interrupted.
		refs := runGit(t, worktree, "ls-remote", "--heads", "origin")
		if !strings.Contains(refs, "bk/preflight/cancel-a") {
			t.Error("expected cancel-a to be preserved after cancellation")
		}
		if !strings.Contains(refs, "bk/preflight/cancel-b") {
			t.Error("expected cancel-b to be preserved after cancellation")
		}
	})
}

// createPreflightBranch creates a preflight branch on the remote by pushing a commit.
func createPreflightBranch(t *testing.T, worktree, branch string) {
	t.Helper()

	// Create a file and commit it on a temporary local branch.
	file := filepath.Join(worktree, "preflight-marker.txt")
	if err := os.WriteFile(file, []byte(branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "add", "preflight-marker.txt")
	runGit(t, worktree, "commit", "-m", "preflight snapshot for "+branch)

	// Push HEAD to the preflight branch on origin, then reset back.
	commit := runGit(t, worktree, "rev-parse", "HEAD")
	runGit(t, worktree, "push", "origin", commit+":refs/heads/"+branch)
	runGit(t, worktree, "reset", "--hard", "HEAD~1")
}
