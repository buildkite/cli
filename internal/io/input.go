package io

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// HasDataAvailable will return whether the given Reader has data available to read
func HasDataAvailable(reader io.Reader) bool {
	switch f := reader.(type) {
	case *os.File:
		info, err := f.Stat()
		if err != nil {
			return false
		}
		return info.Size() > 0
	case *bufio.Reader:
		return f.Size() > 0
	case *strings.Reader:
		return f.Size() > 0
	}

	return false
}
