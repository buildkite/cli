package output

import "testing"

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  int
		expected string
	}{
		{30, "30s"},
		{90, "1m30s"},
		{60, "1m00s"}, // Bug: actual output is "1m0s"
	}

	for _, tt := range tests {
		got := FormatDuration(tt.seconds, "elapsed")
		if got != tt.expected {
			t.Errorf("FormatDuration(%d) = %q, want %q", tt.seconds, got, tt.expected)
		}
	}
}
