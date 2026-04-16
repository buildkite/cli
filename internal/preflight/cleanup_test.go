package preflight

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/google/uuid"
)

func TestBranchBuild_IsCompleted(t *testing.T) {
	tests := []struct {
		name  string
		build *buildkite.Build
		want  bool
	}{
		{"nil build", nil, true},
		{"passed", &buildkite.Build{State: "passed"}, true},
		{"failed", &buildkite.Build{State: "failed"}, true},
		{"canceled", &buildkite.Build{State: "canceled"}, true},
		{"running", &buildkite.Build{State: "running"}, false},
		{"scheduled", &buildkite.Build{State: "scheduled"}, false},
		{"failing", &buildkite.Build{State: "failing"}, false},
		{"blocked", &buildkite.Build{State: "blocked"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bb := BranchBuild{Branch: "bk/preflight/test", Build: tt.build}
			if got := bb.IsCompleted(); got != tt.want {
				t.Errorf("IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestCleanup_AlreadyDeleted(t *testing.T) {
	worktree := initTestRepo(t)

	preflightID := uuid.MustParse("00000000-0000-0000-0000-000000000011")
	result, err := Snapshot(worktree, preflightID)
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	// Delete the branch manually first.
	runGit(t, worktree, "push", "origin", "--delete", result.Ref)

	// Cleanup should succeed even though the branch is already gone.
	if err := Cleanup(worktree, result.Ref, false); err != nil {
		t.Fatalf("Cleanup() should succeed when branch already deleted, got: %v", err)
	}
}
