package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Snapshot creates a temporary commit from the current working tree (including
// uncommitted changes) without touching the real git index, then pushes it to
// the given remote branch. It returns the full commit SHA.
//
// This mirrors the process used by the precommit tool:
//  1. Create a temp index seeded from HEAD
//  2. Stage the entire worktree into the temp index
//  3. Write a tree object
//  4. Create a commit on top of HEAD
//  5. Push the detached commit to refs/heads/bk-preflight/<preflight-id>
func Snapshot(branch string, preflight_id string) (string, error) {
	tmp, err := os.CreateTemp("", "git-index-*")
	if err != nil {
		return "", fmt.Errorf("create temp index: %w", err)
	}
	tmpIndex := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpIndex)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndex)

	// Seed the temp index from HEAD.
	if err := run(env, "git", "read-tree", "HEAD"); err != nil {
		return "", err
	}

	// Stage the entire worktree into the temp index.
	if err := run(env, "git", "add", "-A"); err != nil {
		return "", err
	}

	// Write the tree object.
	tree, err := runOut(env, "git", "write-tree")
	if err != nil {
		return "", err
	}

	// Create a commit on top of HEAD.
	commit, err := runOut(env, "git", "commit-tree", tree, "-p", "HEAD", "-m", fmt.Sprintf("Preflight snapshot for %s", branch))
	if err != nil {
		return "", err
	}

	// Push the detached commit to the remote branch.
	refspec := commit + ":refs/heads/bk-preflight/" + preflight_id
	if err := run(env, "git", "push", "--force", "origin", refspec); err != nil {
		return "", err
	}

	return commit, nil
}

func run(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func runOut(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
