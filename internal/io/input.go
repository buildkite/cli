package io

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// HasDataAvailable will return whether the given Reader has data available to read
func HasDataAvailable(reader io.Reader) bool {
	switch f := reader.(type) {
	case *os.File:
		return !isatty.IsTerminal(f.Fd()) && !isatty.IsCygwinTerminal(f.Fd())
	case *bufio.Reader:
		return f.Size() > 0
	case *strings.Reader:
		return f.Size() > 0
	}

	return false
}
