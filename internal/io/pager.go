package io

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anmitsu/go-shlex"
	"github.com/mattn/go-isatty"
)

// Pager returns a writer hooked up to a pager (default: less -R) when stdout is a TTY.
// Falls back to stdout when paging is disabled or the pager cannot run.
// If pagerCmd is provided, it takes precedence over the PAGER environment variable.
func Pager(noPager bool, pagerCmd ...string) (w io.Writer, cleanup func() error) {
	cleanup = func() error { return nil }

	if noPager || !isTTY() {
		return os.Stdout, cleanup
	}

	// Determine pager command: explicit arg > PAGER env > default
	var pagerEnv string
	if len(pagerCmd) > 0 && pagerCmd[0] != "" {
		pagerEnv = pagerCmd[0]
	} else {
		pagerEnv = os.Getenv("PAGER")
	}
	if pagerEnv == "" {
		pagerEnv = "less -R"
	}

	parts, err := shlex.Split(pagerEnv, true)
	if err != nil || len(parts) == 0 {
		return os.Stdout, cleanup
	}

	pagerBin := parts[0]
	pagerArgs := parts[1:]

	pagerPath, err := exec.LookPath(pagerBin)
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
		return os.Stdout, func() error { return nil }
	}

	var once sync.Once
	var cleanupErr error

	cleanup = func() error {
		once.Do(func() {
			closeErr := stdin.Close()
			waitErr := cmd.Wait()

			if closeErr != nil {
				cleanupErr = closeErr
			} else {
				cleanupErr = waitErr
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
	base := filepath.Base(path)
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
