package preflight

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
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

	preflightID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
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
	remoteCommit := runGit(t, worktree, "ls-remote", "origin", result.Ref)
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

	preflightID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
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

	_, err := Snapshot(worktree, uuid.MustParse("00000000-0000-0000-0000-000000000003"))
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// The real index should be unchanged.
	statusAfter := runGit(t, worktree, "status", "--porcelain")
	if statusBefore != statusAfter {
		t.Errorf("git status changed after Snapshot:\nbefore: %q\nafter:  %q", statusBefore, statusAfter)
	}
}

func TestSnapshot_UniquePreflightIDs(t *testing.T) {
	worktree := initTestRepo(t)

	// First snapshot.
	result1, err := Snapshot(worktree, uuid.MustParse("00000000-0000-0000-0000-000000000004"))
	if err != nil {
		t.Fatalf("first Snapshot() error: %v", err)
	}

	// Modify a file and snapshot with a different preflight ID.
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result2, err := Snapshot(worktree, uuid.MustParse("00000000-0000-0000-0000-000000000005"))
	if err != nil {
		t.Fatalf("second Snapshot() error: %v", err)
	}

	if result1.Commit == result2.Commit {
		t.Error("expected different commits for different snapshots")
	}

	// Both remote branches should exist with their respective commits.
	remote1 := runGit(t, worktree, "ls-remote", "origin", result1.Ref)
	if !strings.Contains(remote1, result1.Commit) {
		t.Errorf("run-1 branch should point to %s, got %q", result1.Commit, remote1)
	}

	remote2 := runGit(t, worktree, "ls-remote", "origin", result2.Ref)
	if !strings.Contains(remote2, result2.Commit) {
		t.Errorf("run-2 branch should point to %s, got %q", result2.Commit, remote2)
	}
}

func TestSnapshotResult_ShortCommit(t *testing.T) {
	tests := []struct {
		name   string
		commit string
		want   string
	}{
		{
			name:   "full SHA is truncated to 10 chars",
			commit: "abc123def456789000aabbccddeeff0011223344",
			want:   "abc123def4",
		},
		{
			name:   "exactly 10 chars",
			commit: "abc123def4",
			want:   "abc123def4",
		},
		{
			name:   "short commit returned as-is",
			commit: "abc",
			want:   "abc",
		},
		{
			name:   "empty commit",
			commit: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := SnapshotResult{Commit: tt.commit}
			if got := r.ShortCommit(); got != tt.want {
				t.Errorf("ShortCommit() = %q, want %q", got, tt.want)
			}
		})
	}
}

// setupDiffEnv creates a temp git index seeded from HEAD and returns the env
// slice for use with diffFiles. The caller can stage changes into this index
// using git commands with the returned env.
func setupDiffEnv(t *testing.T, worktree string) []string {
	t.Helper()

	tmp, err := os.CreateTemp("", "git-index-test-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpIndex := tmp.Name()
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmpIndex) })

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndex)

	cmd := exec.Command("git", "read-tree", "HEAD")
	cmd.Dir = worktree
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git read-tree HEAD: %v\n%s", err, out)
	}

	return env
}

func TestDiffFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, worktree string, env []string)
		want  []FileChange
	}{
		{
			name: "modified file",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# changed\n"), 0o644)
				cmd := exec.Command("git", "add", "README.md")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{{Status: "M", Path: "README.md"}},
		},
		{
			name: "added file",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("new\n"), 0o644)
				cmd := exec.Command("git", "add", "new.txt")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{{Status: "A", Path: "new.txt"}},
		},
		{
			name: "deleted file",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.Remove(filepath.Join(worktree, "README.md"))
				cmd := exec.Command("git", "add", "README.md")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{{Status: "D", Path: "README.md"}},
		},
		{
			name: "renamed file",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.Rename(filepath.Join(worktree, "README.md"), filepath.Join(worktree, "DOCS.md"))
				cmd := exec.Command("git", "add", "-A")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{{Status: "R", Path: "DOCS.md"}},
		},
		{
			name: "file with spaces in name",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.WriteFile(filepath.Join(worktree, "my file.txt"), []byte("data\n"), 0o644)
				cmd := exec.Command("git", "add", "my file.txt")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{{Status: "A", Path: "my file.txt"}},
		},
		{
			name: "no changes",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
			},
			want: nil,
		},
		{
			name: "multiple changes",
			setup: func(t *testing.T, worktree string, env []string) {
				t.Helper()
				os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# v2\n"), 0o644)
				os.WriteFile(filepath.Join(worktree, "a.txt"), []byte("a\n"), 0o644)
				os.WriteFile(filepath.Join(worktree, "b.txt"), []byte("b\n"), 0o644)
				cmd := exec.Command("git", "add", "-A")
				cmd.Dir = worktree
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v\n%s", err, out)
				}
			},
			want: []FileChange{
				{Status: "M", Path: "README.md"},
				{Status: "A", Path: "a.txt"},
				{Status: "A", Path: "b.txt"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktree := initTestRepo(t)
			env := setupDiffEnv(t, worktree)
			tt.setup(t, worktree, env)

			got, err := diffFiles(worktree, env, false)
			if err != nil {
				t.Fatalf("diffFiles() error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("diffFiles() returned %d files, want %d\ngot: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i].Status != tt.want[i].Status {
					t.Errorf("file[%d].Status = %q, want %q", i, got[i].Status, tt.want[i].Status)
				}
				if got[i].Path != tt.want[i].Path {
					t.Errorf("file[%d].Path = %q, want %q", i, got[i].Path, tt.want[i].Path)
				}
			}
		})
	}
}

func TestSnapshot_CleanWorktree(t *testing.T) {
	worktree := initTestRepo(t)

	preflightID := uuid.MustParse("00000000-0000-0000-0000-000000000006")
	result, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// Should push HEAD directly with no files changed.
	head := runGit(t, worktree, "rev-parse", "HEAD")
	if result.Commit != head {
		t.Errorf("expected HEAD %s, got %s", head, result.Commit)
	}

	if len(result.Files) != 0 {
		t.Errorf("expected no changed files, got %d", len(result.Files))
	}

	// The remote branch should exist and point to HEAD.
	remoteRef := runGit(t, worktree, "ls-remote", "origin", result.Ref)
	if !strings.Contains(remoteRef, head) {
		t.Errorf("remote branch should point to HEAD %s, got %q", head, remoteRef)
	}
}
