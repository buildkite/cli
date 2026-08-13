//go:build !windows

package job

import (
	"os"
	"os/signal"
	"syscall"
)

func terminalSizeFile(input, _ *os.File) *os.File {
	return input
}

func startWindowSizeNotifications(_ *os.File, _, _ int, ch chan<- os.Signal) func() {
	signal.Notify(ch, syscall.SIGWINCH)
	return func() {
		signal.Stop(ch)
	}
}
