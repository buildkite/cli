package io

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/anmitsu/go-shlex"
	"github.com/mattn/go-isatty"
)

// Pager returns a writer hooked up to a pager (default: less -R) when stdout is a TTY.
// Falls back to stdout when paging is disabled or the pager cannot run.
func Pager(noPager bool) (w io.Writer, cleanup func() error) {
	cleanup = func() error { return nil }

	if noPager || !isTTY() {
		return os.Stdout, cleanup
	}

	pagerEnv := os.Getenv("PAGER")
	if pagerEnv == "" {
		pagerEnv = "less -R"
	}

	parts, err := shlex.Split(pagerEnv, true)
	if err != nil || len(parts) == 0 {
		return os.Stdout, cleanup
	}

	pagerCmd := parts[0]
	pagerArgs := parts[1:]

	pagerPath, err := exec.LookPath(pagerCmd)
	if err != nil {
		return os.Stdout, cleanup
	}

	if isLessPager(pagerPath) && !hasFlag(pagerArgs, "-R", "--RAW-CONTROL-CHARS") {
		pagerArgs = append(pagerArgs, "-R")
	}

	cmd := exec.Command(pagerPath, pagerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return os.Stdout, cleanup
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return os.Stdout, cleanup
	}

	var once sync.Once
	var cleanupErr error

	cleanup = func() error {
		once.Do(func() {
			closeErr := stdin.Close()
			waitErr := cmd.Wait()

			if waitErr != nil {
				cleanupErr = waitErr
			} else {
				cleanupErr = closeErr
			}
		})
		return cleanupErr
	}

	return stdin, cleanup
}

func isTTY() bool {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		return true
	}
	return isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func isLessPager(path string) bool {
	base := path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		base = path[idx+1:]
	}
	if idx := strings.LastIndex(path, "\\"); idx != -1 {
		base = path[idx+1:]
	}
	return base == "less" || base == "less.exe"
}

func hasFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true
			}
		}
	}
	return false
}
