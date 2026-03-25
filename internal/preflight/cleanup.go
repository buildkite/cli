package preflight

import "fmt"

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
