package preflight

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"

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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline"}
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

		cmd := &PreflightCmd{Pipeline: "test-org/test-pipeline"}
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
