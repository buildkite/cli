package job

import "testing"

func TestStripTimestamps(t *testing.T) {
	t.Parallel()

	in := "\x1b_bk;t=1700000000000\x07hello"
	if got := stripTimestamps(in); got != "hello" {
		t.Errorf("stripTimestamps(%q) = %q, want %q", in, got, "hello")
	}
}
