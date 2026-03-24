package preflight

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	t.Run("succeeds with dirty worktree", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		// Create a dirty file so the snapshot has something to commit.
		if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := &PreflightCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("succeeds with clean worktree", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")

		worktree := initTestRepo(t)
		t.Chdir(worktree)

		cmd := &PreflightCmd{}
		err := cmd.Run(nil, stubGlobals{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
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
