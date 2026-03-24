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

// Screen manages multiple named regions on a terminal, each independently
// rewritable. Regions are rendered top-to-bottom in insertion order.
//
// Usage:
//
//	s := NewScreen(os.Stdout)
//	completed := s.AddRegion("completed")
//	jobs := s.AddRegion("jobs")
//	status := s.AddRegion("status")
//
//	jobs.SetLines([]string{"  ● lint  running", "  ● test  running"})
//	status.SetLines([]string{"  ⠋ Watching…"})
//
//	// Promote a completed job:
//	completed.AppendLine("  ✓ lint  passed")
//
//	// Update active jobs:
//	jobs.SetLines([]string{"  ● test  running"})
type Screen struct {
	mu      sync.Mutex
	w       *os.File
	regions []*Region
	total   int // total lines currently on screen
}

// Region is a named section of a Screen that can be independently updated.
// All methods are safe for concurrent use.
type Region struct {
	screen *Screen
	idx    int
	id     string
	lines  []string
}

// NewScreen creates a Screen that writes to the given file (typically
// os.Stdout).
func NewScreen(w *os.File) *Screen {
	return &Screen{w: w}
}

// AddRegion appends a new named region to the bottom of the screen and returns
// it. Regions render in insertion order. Must be called before any updates.
func (s *Screen) AddRegion(id string) *Region {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := &Region{
		screen: s,
		idx:    len(s.regions),
		id:     id,
	}
	s.regions = append(s.regions, r)
	return r
}

// Flush resets all tracking. After Flush, the screen will not overwrite any
// previously rendered lines.
func (s *Screen) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.regions {
		r.lines = nil
	}
	s.total = 0
}

// SetLines replaces the content of this region and redraws from this region
// downward. Regions above are untouched.
func (r *Region) SetLines(lines []string) {
	s := r.screen
	s.mu.Lock()
	defer s.mu.Unlock()

	oldCount := s.regionOldCount(r.idx)
	r.lines = lines

	s.redrawFrom(r.idx, oldCount)
}

// AppendLine adds a line to this region and redraws from this region downward.
func (r *Region) AppendLine(line string) {
	s := r.screen
	s.mu.Lock()
	defer s.mu.Unlock()

	oldCount := len(r.lines)
	r.lines = append(r.lines, line)

	s.redrawFrom(r.idx, oldCount)
}

// redrawFrom redraws all regions from idx onward, given the old line count
// of the target region before mutation. Caller must hold mu.
func (s *Screen) redrawFrom(idx int, oldCount int) {
	// Calculate how many lines exist from this region onward (using old count
	// for the target region, current count for regions below).
	linesFromHere := oldCount
	for i := idx + 1; i < len(s.regions); i++ {
		linesFromHere += len(s.regions[i].lines)
	}

	// Cursor up to the start of the target region.
	for i := 0; i < linesFromHere; i++ {
		fmt.Fprint(s.w, ansiCursorUp)
	}

	// Rewrite from the target region downward.
	newTotal := s.linesAbove(idx)
	for i := idx; i < len(s.regions); i++ {
		for _, line := range s.regions[i].lines {
			fmt.Fprint(s.w, ansiCursorStart+ansiClearLine+line+"\n")
			newTotal++
		}
	}

	// Clear leftover rows if the new total is smaller.
	for i := newTotal; i < s.total; i++ {
		fmt.Fprint(s.w, ansiCursorStart+ansiClearLine+"\n")
	}
	for i := newTotal; i < s.total; i++ {
		fmt.Fprint(s.w, ansiCursorUp)
	}

	s.total = newTotal
}

// linesAbove returns the total line count of all regions above idx.
func (s *Screen) linesAbove(idx int) int {
	n := 0
	for i := 0; i < idx; i++ {
		n += len(s.regions[i].lines)
	}
	return n
}

// regionOldCount returns the current on-screen line count for a region by
// subtracting all other regions from the total.
func (s *Screen) regionOldCount(idx int) int {
	above := s.linesAbove(idx)
	below := 0
	for i := idx + 1; i < len(s.regions); i++ {
		below += len(s.regions[i].lines)
	}
	old := s.total - above - below
	if old < 0 {
		old = 0
	}
	return old
}
