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
		// Check if there are unread bytes remaining
		return f.Len() > 0
	default:
		// For other reader types, we can't easily determine if data is available
		// This is a conservative approach - assume no data is available
		return false
	}
}
