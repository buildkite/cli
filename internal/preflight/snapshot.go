package preflight

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
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

type snapshotConfig struct {
	debug bool
}

// SnapshotOption configures Snapshot behavior.
type SnapshotOption func(*snapshotConfig)

// WithDebug enables verbose git output on failure.
func WithDebug() SnapshotOption {
	return func(cfg *snapshotConfig) { cfg.debug = true }
}

// Snapshot pushes the current working tree state to a remote preflight ref.
// If there are uncommitted changes, it creates a temporary commit containing
// them without touching the real git index. If the worktree is clean, it
// pushes HEAD directly.
func Snapshot(dir string, preflightID uuid.UUID, opts ...SnapshotOption) (*SnapshotResult, error) {
	cfg := &snapshotConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	tmp, err := os.CreateTemp("", "git-index-*")
	if err != nil {
		return nil, fmt.Errorf("create temp index: %w", err)
	}
	tmpIndex := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpIndex)

	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_INDEX_FILE=") {
			env = append(env, e)
		}
	}
	env = append(env, fmt.Sprintf("GIT_INDEX_FILE=%s", tmpIndex))

	// Seed the temp index from HEAD.
	if err := gitRun(dir, env, cfg.debug, "read-tree", "HEAD"); err != nil {
		return nil, err
	}

	// Stage the entire worktree into the temp index.
	if err := gitRun(dir, env, cfg.debug, "add", "-A"); err != nil {
		return nil, err
	}

	// Diff the temp index against HEAD to find changed files.
	files, err := diffFiles(dir, env, cfg.debug)
	if err != nil {
		return nil, err
	}

	head, err := gitOutput(dir, env, cfg.debug, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}

	ref := fmt.Sprintf("refs/heads/bk/preflight/%s", preflightID.String())
	commit := head

	if len(files) > 0 {
		// Write the tree object.
		tree, err := gitOutput(dir, env, cfg.debug, "write-tree")
		if err != nil {
			return nil, err
		}

		// Create a commit on top of HEAD.
		msg := fmt.Sprintf("Preflight snapshot\n\nPreflight Run ID: %s\nBase Commit: %s", preflightID, head)
		commit, err = gitOutput(dir, env, cfg.debug, "commit-tree", tree, "-p", head, "-m", msg)
		if err != nil {
			return nil, err
		}
	}

	// Push the commit to the remote branch.
	refspec := fmt.Sprintf("%s:%s", commit, ref)
	if err := gitRun(dir, env, cfg.debug, "push", "origin", refspec); err != nil {
		return nil, err
	}

	return &SnapshotResult{
		Commit: commit,
		Ref:    ref,
		Files:  files,
	}, nil
}

// diffFiles returns the list of files changed between HEAD and the temp index.
// It uses -z for null-terminated output to correctly handle renames, copies,
// and filenames containing spaces or special characters.
func diffFiles(dir string, env []string, debug bool) ([]FileChange, error) {
	out, err := gitOutput(dir, env, debug, "diff-index", "--cached", "--name-status", "-z", "-M", "HEAD")
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	// With -z, git outputs NUL-separated tokens:
	//   status \0 path \0           — for M, A, D, etc.
	//   status \0 old_path \0 new_path \0  — for R (rename) and C (copy)
	tokens := strings.Split(out, "\x00")
	var files []FileChange
	for i := 0; i < len(tokens); i++ {
		status := tokens[i]
		if status == "" {
			continue
		}
		code := status[:1]
		i++
		if i >= len(tokens) {
			break
		}

		path := tokens[i]
		if code == "R" || code == "C" {
			// Skip old path, use the new path.
			i++
			if i >= len(tokens) {
				break
			}
			path = tokens[i]
		}

		files = append(files, FileChange{
			Status: code,
			Path:   path,
		})
	}

	return files, nil
}
