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
	if f.Quiet || !isatty.IsTerminal(os.Stdout.Fd()) {
		action()
		return nil
	}

	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		chars := []string{".  ", ".. ", "..."}
		maxLen := 0

		for {
			select {
			case <-done:
				// Clear the line by overwriting with spaces
				clearLine := "\r" + fmt.Sprintf("%*s", maxLen, "") + "\r"
				fmt.Fprint(os.Stderr, clearLine)
				close(finished)
				return
			case <-ticker.C:
				line := fmt.Sprintf("%s %s", name, chars[i%len(chars)])
				if len(line) > maxLen {
					maxLen = len(line)
				}
				fmt.Fprintf(os.Stderr, "\r%s", line)
				i++
			}
		}
	}()

	action()
	close(done)
	<-finished // Wait for spinner to finish clearing

	return nil
}
