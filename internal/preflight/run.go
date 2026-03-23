package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitCmd creates an exec.Command for git with the given dir and env pre-configured.
func gitCmd(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd
}

// gitRun runs a git command, streaming output to stderr.
func gitRun(dir string, env []string, args ...string) error {
	cmd := gitCmd(dir, env, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// gitRunQuiet runs a git command, suppressing output unless it fails.
func gitRunQuiet(dir string, env []string, args ...string) error {
	cmd := gitCmd(dir, env, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Stderr.Write(out)
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// gitOutput runs a git command and returns its trimmed stdout.
func gitOutput(dir string, env []string, args ...string) (string, error) {
	cmd := gitCmd(dir, env, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
