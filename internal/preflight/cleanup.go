package preflight

import (
	"fmt"
	"strings"
)

// Cleanup deletes the preflight branch from the remote.
// If the branch no longer exists on the remote, it is treated as success.
func Cleanup(dir string, ref string, debug bool) error {
	out, err := gitOutput(dir, nil, debug, "ls-remote", "origin", ref)
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}

	refspec := fmt.Sprintf(":%s", ref)
	return gitRun(dir, nil, debug, "push", "origin", refspec)
}

// CleanupRefs deletes multiple refs from the remote in a single git push.
// Refs that no longer exist on the remote are silently ignored.
func CleanupRefs(dir string, refs []string, debug bool) error {
	if len(refs) == 0 {
		return nil
	}

	out, err := gitOutput(dir, nil, debug, "ls-remote", "origin", "refs/heads/bk/preflight/*")
	if err != nil {
		return err
	}

	remote := make(map[string]struct{})
	for line := range strings.SplitSeq(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			remote[parts[1]] = struct{}{}
		}
	}

	args := make([]string, 0, 2+len(refs))
	args = append(args, "push", "origin")
	for _, ref := range refs {
		if _, exists := remote[ref]; exists {
			args = append(args, fmt.Sprintf(":%s", ref))
		}
	}

	if len(args) == 2 {
		return nil
	}

	return gitRun(dir, nil, debug, args...)
}
