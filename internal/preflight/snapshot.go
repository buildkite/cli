package preflight

import (
	"fmt"
	"os"
	"strings"
)

// FileChange represents a single file changed in the snapshot.
type FileChange struct {
	Status string // e.g. "M", "A", "D", "R"
	Path   string
}

// SnapshotResult holds the output of a successful snapshot operation.
type SnapshotResult struct {
	Commit string
	Ref    string
	Files  []FileChange
}

// StatusSymbol returns a human-readable symbol for the file change status.
func (f FileChange) StatusSymbol() string {
	switch f.Status {
	case "A":
		return "+"
	case "D":
		return "-"
	default:
		return "~"
	}
}

// Snapshot creates a temporary commit from the current working tree (including
// uncommitted changes) without touching the real git index, then pushes it to
// the given remote branch. It returns the snapshot result including the commit
// SHA and list of changed files.
//
//  1. Create a temp index seeded from HEAD
//  2. Stage the entire worktree into the temp index
//  3. Diff the temp index against HEAD to find changed files
//  4. Write a tree object
//  5. Create a commit on top of HEAD
//  6. Push the detached commit to refs/heads/bk-preflight/<preflight-id>
func Snapshot(dir string, preflightID string) (*SnapshotResult, error) {
	tmp, err := os.CreateTemp("", "git-index-*")
	if err != nil {
		return nil, fmt.Errorf("create temp index: %w", err)
	}
	tmpIndex := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpIndex)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndex)

	// Seed the temp index from HEAD.
	if err := run(dir, env, "git", "read-tree", "HEAD"); err != nil {
		return nil, err
	}

	// Stage the entire worktree into the temp index.
	if err := run(dir, env, "git", "add", "-A"); err != nil {
		return nil, err
	}

	// Diff the temp index against HEAD to find changed files.
	files, err := diffFiles(dir, env)
	if err != nil {
		return nil, err
	}

	// Write the tree object.
	tree, err := runOut(dir, env, "git", "write-tree")
	if err != nil {
		return nil, err
	}

	// Create a commit on top of HEAD.
	commit, err := runOut(dir, env, "git", "commit-tree", tree, "-p", "HEAD", "-m", fmt.Sprintf("Preflight snapshot\n\nPreflight Run ID: %s", preflightID))
	if err != nil {
		return nil, err
	}

	// Push the detached commit to the remote branch.
	ref := "refs/heads/bk-preflight/" + preflightID
	refspec := commit + ":" + ref
	if err := runQuiet(dir, env, "git", "push", "--force", "origin", refspec); err != nil {
		return nil, err
	}

	return &SnapshotResult{
		Commit: commit,
		Ref:    ref,
		Files:  files,
	}, nil
}

// diffFiles returns the list of files changed between HEAD and the temp index.
func diffFiles(dir string, env []string) ([]FileChange, error) {
	out, err := runOut(dir, env, "git", "diff-index", "--cached", "--name-status", "HEAD")
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	var files []FileChange
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		files = append(files, FileChange{
			Status: parts[0][:1], // Take first char (e.g. "R100" → "R")
			Path:   parts[1],
		})
	}

	return files, nil
}


