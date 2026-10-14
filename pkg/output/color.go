package output

import (
	"io"
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
		colorEnabled = ColorEnabledFor(os.Stdout)
	})
	return colorEnabled
}

// ColorEnabledFor checks the actual destination, independently of stdout.
// Non-file writers and redirected streams always receive plain text.
func ColorEnabledFor(w io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
}
