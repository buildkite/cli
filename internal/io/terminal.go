package io

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

const clearPreviousLineANSI = "\x1b[1A\r\x1b[2K"

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func terminalWidth(f *os.File) int {
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

func clearPreviousLines(f *os.File, lines int) {
	if lines <= 0 || !isTerminal(f) {
		return
	}

	for range lines {
		fmt.Fprint(f, clearPreviousLineANSI)
	}
}
