//go:build windows

package job

import (
	"os"
	"time"

	"golang.org/x/term"
)

const windowSizePollInterval = 250 * time.Millisecond

type windowSizeSignal struct{}

func (windowSizeSignal) Signal() {}

func (windowSizeSignal) String() string { return "window size changed" }

func terminalSizeFile(_, output *os.File) *os.File {
	return output
}

func startWindowSizeNotifications(terminal *os.File, width, height int, ch chan<- os.Signal) func() {
	// Windows has no SIGWINCH. Poll the console output handle instead of reading
	// resize events from the input buffer, which is also carrying SSH input.
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(windowSizePollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				newWidth, newHeight, err := term.GetSize(int(terminal.Fd()))
				if err != nil || newWidth == width && newHeight == height {
					continue
				}
				width, height = newWidth, newHeight
				select {
				case ch <- windowSizeSignal{}:
				default:
				}
			case <-stop:
				return
			}
		}
	}()

	return func() {
		close(stop)
		<-stopped
	}
}
