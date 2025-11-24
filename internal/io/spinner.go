package io

import (
	"fmt"
	"os"
	"time"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/mattn/go-isatty"
)

func SpinWhile(f *factory.Factory, name string, action func()) error {
	// If quiet mode is on or not a terminal, just run the action
	if f.Quiet || !isatty.IsTerminal(os.Stderr.Fd()) {
		action()
		return nil
	}

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		chars := []string{".  ", ".. ", "..."}

		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r%s... Done\n", name)
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", name, chars[i%len(chars)])
				i++
			}
		}
	}()

	action()
	close(done)

	return nil
}
