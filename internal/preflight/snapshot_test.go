package preflight

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a real git repository in a temp directory with an
// initial commit and a bare "origin" remote so that Snapshot can push.
// It returns the worktree path and a cleanup-aware test helper.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	worktree := filepath.Join(dir, "work")
	bare := filepath.Join(dir, "origin.git")

	// Create the bare remote.
	runGit(t, "", "init", "--bare", bare)

	// Create the working repo.
	runGit(t, "", "init", worktree)
	runGit(t, worktree, "config", "user.email", "test@test.com")
	runGit(t, worktree, "config", "user.name", "Test")

	// Create an initial commit so HEAD exists.
	initial := filepath.Join(worktree, "README.md")
	if err := os.WriteFile(initial, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "add", ".")
	runGit(t, worktree, "commit", "-m", "initial commit")

	// Add the bare repo as origin.
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

func TestSnapshot_CommittedChanges(t *testing.T) {

	worktree := initTestRepo(t)

	// Add a tracked file change (but don't commit it).
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	preflightID := "test-id-committed"
	result, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	if len(result.Commit) != 40 {
		t.Errorf("expected 40-char SHA, got %q (len %d)", result.Commit, len(result.Commit))
	}

	// The commit should exist in the repo.
	runGit(t, worktree, "cat-file", "-t", result.Commit)

	// The snapshot tree should contain the updated content.
	content := runGit(t, worktree, "show", result.Commit+":README.md")
	if content != "# updated" {
		t.Errorf("snapshot content = %q, want %q", content, "# updated")
	}

	// The remote branch should have been pushed.
	remoteCommit := runGit(t, worktree, "ls-remote", "origin", "refs/heads/bk-preflight/"+preflightID)
	if !strings.Contains(remoteCommit, result.Commit) {
		t.Errorf("remote branch does not contain commit %s, got %q", result.Commit, remoteCommit)
	}
}

func TestSnapshot_UntrackedFiles(t *testing.T) {

	worktree := initTestRepo(t)

	// Add a brand new untracked file.
	if err := os.WriteFile(filepath.Join(worktree, "new-file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	preflightID := "test-id-untracked"
	result, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// The snapshot should include the untracked file.
	content := runGit(t, worktree, "show", result.Commit+":new-file.txt")
	if content != "hello" {
		t.Errorf("untracked file content = %q, want %q", content, "hello")
	}
}

func TestSnapshot_DoesNotModifyRealIndex(t *testing.T) {

	worktree := initTestRepo(t)

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(worktree, "untracked.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Record the index state before snapshot.
	statusBefore := runGit(t, worktree, "status", "--porcelain")

	_, err := Snapshot(worktree, "test-id-index")
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// The real index should be unchanged.
	statusAfter := runGit(t, worktree, "status", "--porcelain")
	if statusBefore != statusAfter {
		t.Errorf("git status changed after Snapshot:\nbefore: %q\nafter:  %q", statusBefore, statusAfter)
	}
}

func TestSnapshot_ForcePushesExistingBranch(t *testing.T) {

	worktree := initTestRepo(t)

	preflightID := "test-id-force"

	// First snapshot.
	result1, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("first Snapshot() error: %v", err)
	}

	// Modify a file and snapshot again to the same branch.
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result2, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("second Snapshot() error: %v", err)
	}

	if result1.Commit == result2.Commit {
		t.Error("expected different commits for different snapshots")
	}

	// The remote branch should point to the second commit.
	remoteRef := runGit(t, worktree, "ls-remote", "origin", "refs/heads/bk-preflight/"+preflightID)
	if !strings.Contains(remoteRef, result2.Commit) {
		t.Errorf("remote branch should point to %s, got %q", result2.Commit, remoteRef)
	}
}
