package preflight

import "fmt"

// Cleanup deletes the preflight branch from the remote.
func Cleanup(dir string, ref string, debug bool) error {
	refspec := fmt.Sprintf(":%s", ref)
	return gitRun(dir, nil, debug, "push", "origin", refspec)
}
