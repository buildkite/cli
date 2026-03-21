package preflight

import (
	"fmt"
	"os"
	"sync"
)

const (
	ansiCursorUp    = "\033[A"
	ansiClearLine   = "\033[2K"
	ansiCursorStart = "\r"
)

// LiveWriter manages an in-place terminal region that can be overwritten on
// each update, while allowing finished lines to be "promoted" into permanent
// scrollback above the live region.
type LiveWriter struct {
	mu        sync.Mutex
	w         *os.File
	lines     []string
	lineCount int
}

// NewLiveWriter creates a LiveWriter that writes to the given file (typically
// os.Stdout).
func NewLiveWriter(w *os.File) *LiveWriter {
	return &LiveWriter{w: w}
}

// SetLines replaces the live region with the given lines.
func (lw *LiveWriter) SetLines(lines []string) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Move cursor up to the start of the previous live region.
	for i := 0; i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorUp)
	}
	// Write new lines, clearing each row first.
	for _, line := range lines {
		fmt.Fprint(lw.w, ansiCursorStart+ansiClearLine+line+"\n")
	}
	// Clear any leftover rows from the previous render.
	for i := len(lines); i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorStart+ansiClearLine+"\n")
	}
	// Move cursor back up over the cleared leftovers.
	for i := len(lines); i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorUp)
	}

	lw.lines = lines
	lw.lineCount = len(lines)
}

// Flush resets the live region tracking so nothing is overwritten on the next
// call to SetLines.
func (lw *LiveWriter) Flush() {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.lineCount = 0
	lw.lines = nil
}

// Println writes a permanent line above the current live region, then redraws
// the live region below it. This is used to promote completed jobs into
// scrollback.
func (lw *LiveWriter) Println(msg string) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Clear the live region.
	for i := 0; i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorUp)
	}
	for i := 0; i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorStart+ansiClearLine+"\n")
	}
	for i := 0; i < lw.lineCount; i++ {
		fmt.Fprint(lw.w, ansiCursorUp)
	}

	// Write the permanent line.
	fmt.Fprintln(lw.w, msg)

	// Redraw the live region below.
	for _, line := range lw.lines {
		fmt.Fprint(lw.w, ansiCursorStart+ansiClearLine+line+"\n")
	}
}
