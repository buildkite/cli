package job

import "testing"

func TestFormatForLLM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "strips ANSI color codes",
			input: "\x1b[31mred\x1b[0m text",
			want:  "red text",
		},
		{
			name:  "strips APC timestamp sequences",
			input: "\x1b_bk;t=1700000000000\x07hello",
			want:  "hello",
		},
		{
			name:  "strips bare timestamp markers",
			input: "bk;t=1700000000000\x07hello",
			want:  "hello",
		},
		{
			name:  "handles header after timestamp marker",
			input: "\x1b_bk;t=1700000000000\x07~~~ Preparing secrets\r",
			want:  "\n=== PHASE:  Preparing secrets ===",
		},
		{
			name:  "deduplicates consecutive identical lines",
			input: "loop\nloop\nloop\ndone",
			want:  "loop\n[Previous line repeated 2 times]\ndone",
		},
		{
			name:  "deduplicates run at end of input",
			input: "start\nloop\nloop\nloop",
			want:  "start\nloop\n[Previous line repeated 2 times]",
		},
		{
			name:  "does not deduplicate blank lines",
			input: "a\n\n\n\nb",
			want:  "a\n\n\n\nb",
		},
		{
			name:  "does not deduplicate non-adjacent duplicates",
			input: "a\nb\na",
			want:  "a\nb\na",
		},
		{
			name:  "collapses carriage-return redraws",
			input: "10%\r50%\r100%",
			want:  "100%",
		},
		{
			name:  "trims trailing CR from CRLF line endings",
			input: "hello\r\nworld\r\n",
			want:  "hello\nworld\n",
		},
		{
			name:  "collapses redraws with trailing CRs",
			input: "10%\r50%\r100% done\r\r",
			want:  "100% done",
		},
		{
			name:  "rewrites group markers into phase headers",
			input: "--- Running tests",
			want:  "\n=== PHASE:  Running tests ===",
		},
		{
			name:  "rewrites plus and tilde markers",
			input: "+++ Failed\n~~~ Cleanup",
			want:  "\n=== PHASE:  Failed ===\n\n=== PHASE:  Cleanup ===",
		},
		{
			name:  "leaves separator-like lines intact",
			input: "----------",
			want:  "----------",
		},
		{
			name:  "standalone marker becomes header",
			input: "---",
			want:  "\n=== PHASE:  ===",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatForLLM(tt.input)
			if got != tt.want {
				t.Errorf("formatForLLM(%q) =\n%q\nwant\n%q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripTimestamps(t *testing.T) {
	t.Parallel()

	in := "\x1b_bk;t=1700000000000\x07hello"
	if got := stripTimestamps(in); got != "hello" {
		t.Errorf("stripTimestamps(%q) = %q, want %q", in, got, "hello")
	}
}
