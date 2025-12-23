package io

import "testing"

func TestProgressBar(t *testing.T) {
	t.Parallel()

	bar := ProgressBar(5, 10, 10)
	if bar != "[█████░░░░░]" {
		t.Fatalf("progress bar mismatch: %q", bar)
	}

	empty := ProgressBar(0, 0, 4)
	if empty != "[░░░░]" {
		t.Fatalf("empty bar mismatch: %q", empty)
	}
}

func TestProgressLine(t *testing.T) {
	t.Parallel()

	line := ProgressLine("Work", 3, 10, 2, 1, 6)
	expected := "Work [█░░░░░]  30% 3/10 passed:2 failed:1"
	if line != expected {
		t.Fatalf("progress line mismatch: got %q want %q", line, expected)
	}

	noItems := ProgressLine("Work", 0, 0, 0, 0, 6)
	if noItems != "Work [no items]" {
		t.Fatalf("no items line mismatch: %q", noItems)
	}
}
