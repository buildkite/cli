package io

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal returns true if stdout is a terminal
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}
