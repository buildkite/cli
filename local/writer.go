package local

import (
	"bytes"
	"regexp"
)

type lineWriter struct {
	f       func(line string)
	started bool
	buf     *bytes.Buffer
	offset  int
}

var lineRegexp = regexp.MustCompile(`(?m:^(.*)\r?\n)`)

func newLineWriter(f func(line string)) *lineWriter {
	return &lineWriter{
		f:   f,
		buf: bytes.NewBuffer([]byte("")),
	}
}

func (l *lineWriter) Write(p []byte) (n int, err error) {
	if bytes.ContainsRune(p, '\n') {
		l.started = true
	}

	if n, err = l.buf.Write(p); err != nil {
		return
	}

	matches := lineRegexp.FindAllStringSubmatch(l.buf.String()[l.offset:], -1)

	for _, match := range matches {
		l.f(match[1])
		l.offset += len(match[0])
	}

	return
}

func (l *lineWriter) Close() error {
	if remaining := l.buf.String()[l.offset:]; len(remaining) > 0 {
		l.f(remaining)
	}
	l.buf = bytes.NewBuffer([]byte(""))
	return nil
}
