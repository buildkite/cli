package preflight

import (
	"testing"

	"github.com/google/uuid"
)

func TestCleanup(t *testing.T) {
	worktree := initTestRepo(t)

	preflightID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	result, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// Verify the remote branch exists before cleanup.
	out := runGit(t, worktree, "ls-remote", "origin", result.Ref)
	if out == "" {
		t.Fatal("expected remote branch to exist before cleanup")
	}

	if err := Cleanup(worktree, result.Ref, false); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Verify the remote branch no longer exists.
	out = runGit(t, worktree, "ls-remote", "origin", result.Ref)
	if out != "" {
		t.Errorf("expected remote branch to be deleted, got %q", out)
	}
}
