package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// gitCmd creates an exec.Command for git with the given dir and env pre-configured.
func gitCmd(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd
}

// gitRun runs a git command, discarding output on success.
func gitRun(dir string, env []string, debug bool, args ...string) error {
	cmd := gitCmd(dir, env, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if debug {
			os.Stderr.Write(out)
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// gitOutput runs a git command and returns its trimmed stdout.
func gitOutput(dir string, env []string, debug bool, args ...string) (string, error) {
	cmd := gitCmd(dir, env, args...)
	out, err := cmd.Output()
	if err != nil {
		if debug {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Stderr.Write(ee.Stderr)
			}
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RepositoryRoot returns the top-level path for the git repository containing dir.
func RepositoryRoot(dir string, debug bool) (string, error) {
	return gitOutput(dir, nil, debug, "rev-parse", "--show-toplevel")
}

// SourceContext describes the original git state that preflight was created from.
type SourceContext struct {
	Branch string
	Commit string
}

// ResolveSourceContext returns the current branch name (if any) and HEAD commit.
func ResolveSourceContext(dir string, debug bool) (SourceContext, error) {
	branch, err := gitOutput(dir, nil, debug, "branch", "--show-current")
	if err != nil {
		return SourceContext{}, err
	}

	commit, err := gitOutput(dir, nil, debug, "rev-parse", "HEAD")
	if err != nil {
		return SourceContext{}, err
	}

	return SourceContext{Branch: branch, Commit: commit}, nil
}

// tempIndexEnv returns a copy of the current environment with GIT_INDEX_FILE
// set to path, stripping any existing GIT_INDEX_FILE entry. This is used to
// direct git commands at a temporary index without affecting the real one.
func tempIndexEnv(path string) []string {
	env := slices.DeleteFunc(os.Environ(), func(e string) bool {
		return strings.HasPrefix(e, "GIT_INDEX_FILE=")
	})
	return append(env, "GIT_INDEX_FILE="+path)
}
