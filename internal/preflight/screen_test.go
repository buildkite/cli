package preflight

import (
	"os"
	"strings"
	"testing"
)

// captureScreen runs fn with a Screen wired to a temp file, then returns what
// was written.
func captureScreen(t *testing.T, fn func(s *Screen)) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "screen-*")
	if err != nil {
		t.Fatal(err)
	}

	s := NewScreen(f)
	fn(s)

	f.Close()

	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// stripANSI removes cursor movement and clear-line sequences so we can check
// visible text content.
func stripANSI(s string) string {
	s = strings.ReplaceAll(s, ansiCursorUp, "")
	s = strings.ReplaceAll(s, ansiClearLine, "")
	s = strings.ReplaceAll(s, ansiCursorStart, "")
	return s
}

// visibleLines returns non-empty lines after stripping ANSI.
func visibleLines(raw string) []string {
	stripped := stripANSI(raw)
	var result []string
	for _, line := range strings.Split(stripped, "\n") {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func TestScreen_InitialRender(t *testing.T) {
	raw := captureScreen(t, func(s *Screen) {
		header := s.AddRegion("header")
		body := s.AddRegion("body")
		footer := s.AddRegion("footer")

		header.SetLines([]string{"=== Build #42 ==="})
		body.SetLines([]string{"  ● job1 running", "  ● job2 running"})
		footer.SetLines([]string{"Watching…"})
	})

	lines := visibleLines(raw)

	expected := []string{
		"=== Build #42 ===",
		"  ● job1 running",
		"  ● job2 running",
		"Watching…",
	}
	if len(lines) < len(expected) {
		t.Fatalf("got %d visible lines, want at least %d:\n%s", len(lines), len(expected), raw)
	}
	for i, want := range expected {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
	}
}

func TestScreen_UpdateMiddleRegion(t *testing.T) {
	raw := captureScreen(t, func(s *Screen) {
		header := s.AddRegion("header")
		body := s.AddRegion("body")
		footer := s.AddRegion("footer")

		header.SetLines([]string{"Header"})
		body.SetLines([]string{"old1", "old2"})
		footer.SetLines([]string{"Footer"})

		// Update just the body — header and footer should be unchanged.
		body.SetLines([]string{"new1", "new2"})
	})

	lines := visibleLines(raw)

	// After the update, the last visible occurrence of "new1" and "new2" should
	// appear between "Header" and "Footer".
	last := lines[len(lines)-3:]
	if last[0] != "new1" || last[1] != "new2" || last[2] != "Footer" {
		t.Errorf("final visible lines = %v, want [new1, new2, Footer]", last)
	}
}

func TestScreen_RegionGrows(t *testing.T) {
	raw := captureScreen(t, func(s *Screen) {
		top := s.AddRegion("top")
		bottom := s.AddRegion("bottom")

		top.SetLines([]string{"A"})
		bottom.SetLines([]string{"Z"})

		// Grow the top region.
		top.SetLines([]string{"A", "B", "C"})
	})

	lines := visibleLines(raw)

	// The final state should end with A, B, C, Z.
	tail := lines[len(lines)-4:]
	want := []string{"A", "B", "C", "Z"}
	for i, w := range want {
		if tail[i] != w {
			t.Errorf("tail[%d] = %q, want %q", i, tail[i], w)
		}
	}
}

func TestScreen_RegionShrinks(t *testing.T) {
	raw := captureScreen(t, func(s *Screen) {
		top := s.AddRegion("top")
		bottom := s.AddRegion("bottom")

		top.SetLines([]string{"A", "B", "C"})
		bottom.SetLines([]string{"Z"})

		// Shrink the top region.
		top.SetLines([]string{"A"})
	})

	lines := visibleLines(raw)

	// Final visible state should end with A, Z (extra lines cleared).
	tail := lines[len(lines)-2:]
	if tail[0] != "A" || tail[1] != "Z" {
		t.Errorf("tail = %v, want [A, Z]", tail)
	}
}

func TestScreen_AppendLine(t *testing.T) {
	raw := captureScreen(t, func(s *Screen) {
		log := s.AddRegion("log")
		status := s.AddRegion("status")

		log.SetLines([]string{"line1"})
		status.SetLines([]string{"watching…"})

		log.AppendLine("line2")
	})

	lines := visibleLines(raw)

	// Should end with line1, line2, watching…
	tail := lines[len(lines)-3:]
	want := []string{"line1", "line2", "watching…"}
	for i, w := range want {
		if tail[i] != w {
			t.Errorf("tail[%d] = %q, want %q", i, tail[i], w)
		}
	}
}
