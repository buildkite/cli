package io

import "testing"

func TestWrappedLineCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		width int
		want  int
	}{
		{name: "empty string", input: "", width: 80, want: 1},
		{name: "fits on one line", input: "hello", width: 80, want: 1},
		{name: "wraps across two lines", input: "1234567890", width: 5, want: 2},
		{name: "invalid width falls back to one line", input: "hello", width: 0, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := wrappedLineCount(tt.input, tt.width); got != tt.want {
				t.Fatalf("wrappedLineCount(%q, %d) = %d, want %d", tt.input, tt.width, got, tt.want)
			}
		})
	}
}

func TestRenderedLineCount(t *testing.T) {
	t.Parallel()

	got := renderedLineCount("Select a pipeline", []string{"first", "second"}, "Enter number (1-2): 2", 80)
	want := 4
	if got != want {
		t.Fatalf("renderedLineCount() = %d, want %d", got, want)
	}
}
