package preflight

import (
	"context"
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
	Branch string
	Files  []FileChange
}

func (r SnapshotResult) ShortCommit() string {
	if len(r.Commit) >= 10 {
		return r.Commit[:10]
	}
	return r.Commit
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
// It always creates a distinct commit on top of HEAD (even when the worktree
// is clean) without touching the real git index.
func Snapshot(dir string, preflightID uuid.UUID, opts ...SnapshotOption) (*SnapshotResult, error) {
	return SnapshotContext(context.Background(), dir, preflightID, opts...)
}

// SnapshotContext pushes the current working tree state to a remote preflight
// ref, aborting in-flight git commands when ctx is canceled.
func SnapshotContext(ctx context.Context, dir string, preflightID uuid.UUID, opts ...SnapshotOption) (*SnapshotResult, error) {
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

	env := tempIndexEnv(tmpIndex)

	// Seed the temp index from HEAD.
	if err := gitRunContext(ctx, dir, env, cfg.debug, "read-tree", "HEAD"); err != nil {
		return nil, err
	}

	// Stage the entire worktree into the temp index.
	if err := gitRunContext(ctx, dir, env, cfg.debug, "add", "-A"); err != nil {
		return nil, err
	}

	// Diff the temp index against HEAD to find changed files.
	files, err := diffFilesContext(ctx, dir, env, cfg.debug)
	if err != nil {
		return nil, err
	}

	head, err := gitOutputContext(ctx, dir, env, cfg.debug, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}

	branch := fmt.Sprintf("bk/preflight/%s", preflightID.String())
	ref := fmt.Sprintf("refs/heads/%s", branch)

	// Always write a tree and create a new commit, even when there are no
	// local changes. This ensures the preflight branch always points to a
	// distinct commit (not shared with HEAD), which allows commit statuses to
	// be attributed to the preflight run rather than the base commit.
	tree, err := gitOutputContext(ctx, dir, env, cfg.debug, "write-tree")
	if err != nil {
		return nil, err
	}

	msg := fmt.Sprintf("Preflight snapshot\n\nPreflight Run ID: %s\nBase Commit: %s", preflightID, head)
	commit, err := gitOutputContext(ctx, dir, env, cfg.debug, "commit-tree", tree, "-p", head, "-m", msg)
	if err != nil {
		return nil, err
	}

	// Push the commit to the remote branch.
	refspec := fmt.Sprintf("%s:%s", commit, ref)
	if err := gitRunContext(ctx, dir, env, cfg.debug, "push", "origin", refspec); err != nil {
		return nil, err
	}

	return &SnapshotResult{
		Commit: commit,
		Ref:    ref,
		Branch: branch,
		Files:  files,
	}, nil
}

// diffFiles returns the list of files changed between HEAD and the temp index.
// It uses -z for null-terminated output to correctly handle renames, copies,
// and filenames containing spaces or special characters.
func diffFiles(dir string, env []string, debug bool) ([]FileChange, error) {
	return diffFilesContext(context.Background(), dir, env, debug)
}

func diffFilesContext(ctx context.Context, dir string, env []string, debug bool) ([]FileChange, error) {
	out, err := gitOutputContext(ctx, dir, env, debug, "diff-index", "--cached", "--name-status", "-z", "-M", "HEAD")
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
