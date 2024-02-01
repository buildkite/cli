package io

import (
	"io"
)

func HasDataAvailable(reader io.Reader) bool {
	return false
}
