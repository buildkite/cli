package preflight

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"

	"github.com/buildkite/cli/v3/internal/build/watch"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"

	"github.com/buildkite/cli/v3/internal/cli"
)

type stubGlobals struct{}

func (s stubGlobals) SkipConfirmation() bool { return false }
func (s stubGlobals) DisableInput() bool     { return false }
func (s stubGlobals) IsQuiet() bool          { return false }
func (s stubGlobals) DisablePager() bool     { return false }
func (s stubGlobals) EnableDebug() bool      { return false }

var _ cli.GlobalFlags = stubGlobals{}

func TestPreflightCmd_Run(t *testing.T) {
	t.Run("returns validation error when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")

		cmd := &PreflightCmd{}
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

		cmd := &PreflightCmd{}
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

	t.Run("snapshots and creates build", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		var gotReq buildkite.CreateBuild
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" && strings.Contains(r.URL.Path, "/builds") {
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: false, Interval: 2}
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
		if gotReq.Env["BUILDKITE_PREFLIGHT"] != "true" {
			t.Errorf("expected BUILDKITE_PREFLIGHT=true, got %#v", gotReq.Env)
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01, NoCleanup: true}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Watch: true, Interval: 0.01}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline", Interval: 2}
		err := cmd.Run(nil, stubGlobals{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "creating preflight build") {
			t.Fatalf("expected build creation error, got: %v", err)
		}
	})
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
