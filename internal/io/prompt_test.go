package io

import (
	"strings"
	"testing"
)

func TestPromptClearSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines int
		want  string
	}{
		{name: "zero lines", lines: 0, want: ""},
		{name: "single line", lines: 1, want: clearLineSequence},
		{name: "multiple lines", lines: 3, want: strings.Repeat(clearLineSequence, 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := promptClearSequence(tt.lines); got != tt.want {
				t.Fatalf("promptClearSequence(%d) = %q, want %q", tt.lines, got, tt.want)
			}
		})
	}
}
