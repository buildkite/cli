//go:build !windows

package job

import (
	"os"
	"os/signal"
	"syscall"
)

func startWindowSizeNotifications(ch chan<- os.Signal) func() {
	signal.Notify(ch, syscall.SIGWINCH)
	return func() {
		signal.Stop(ch)
	}
}
