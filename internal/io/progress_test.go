package io

import "testing"

func TestProgressBar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		completed int
		total     int
		width     int
		expected  string
	}{
		{"half filled", 5, 10, 10, "[█████░░░░░]"},
		{"empty with zero total", 0, 0, 4, "[░░░░]"},
		{"full bar", 10, 10, 10, "[██████████]"},
		{"overflow clamped", 15, 10, 10, "[██████████]"},
		{"zero width", 5, 10, 0, "[]"},
		{"negative completed", -5, 10, 10, "[░░░░░░░░░░]"},
		{"one char width", 1, 2, 1, "[░]"},
		{"complete one char", 2, 2, 1, "[█]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ProgressBar(tt.completed, tt.total, tt.width)
			if got != tt.expected {
				t.Errorf("ProgressBar(%d, %d, %d) = %q, want %q",
					tt.completed, tt.total, tt.width, got, tt.expected)
			}
		})
	}
}

func TestProgressLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		label     string
		completed int
		total     int
		succeeded int
		failed    int
		barWidth  int
		expected  string
	}{
		{
			"partial progress",
			"Work", 3, 10, 2, 1, 6,
			"Work [█░░░░░]  30% 3/10 succeeded:2 failed:1",
		},
		{
			"no items",
			"Work", 0, 0, 0, 0, 6,
			"Work [no items]",
		},
		{
			"complete",
			"Task", 10, 10, 10, 0, 10,
			"Task [██████████] 100% 10/10 succeeded:10 failed:0",
		},
		{
			"all failed",
			"Ops", 5, 5, 0, 5, 5,
			"Ops [█████] 100% 5/5 succeeded:0 failed:5",
		},
		{
			"mixed results at 50%",
			"Deploy", 5, 10, 3, 2, 8,
			"Deploy [████░░░░]  50% 5/10 succeeded:3 failed:2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ProgressLine(tt.label, tt.completed, tt.total, tt.succeeded, tt.failed, tt.barWidth)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}
