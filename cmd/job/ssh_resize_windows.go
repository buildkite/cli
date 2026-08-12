//go:build windows

package job

import "os"

func startWindowSizeNotifications(chan<- os.Signal) func() {
	return func() {}
}
