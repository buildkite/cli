package output

import (
	"os"
	"sync"

	"github.com/mattn/go-isatty"
)

var (
	colorOnce    sync.Once
	colorEnabled = true
)

// ColorEnabled returns false when the NO_COLOR environment variable is present
// See https://no-color.org for the convention
func ColorEnabled() bool {
	colorOnce.Do(func() {
		if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
			colorEnabled = false
			return
		}

		if term := os.Getenv("TERM"); term == "dumb" {
			colorEnabled = false
			return
		}

		if !(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
			colorEnabled = false
			return
		}
	})
	return colorEnabled
}
