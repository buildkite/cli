package emoji

import (
	"testing"
)

func TestRender_standardEmoji(t *testing.T) {
	got := Render(":checkered_flag: Feature flags")
	want := "🏁 Feature flags"
	if got != want {
		t.Errorf("Render(%q) = %q, want %q", ":checkered_flag: Feature flags", got, want)
	}
}

func TestRender_plainText(t *testing.T) {
	got := Render("just plain text")
	if got != "just plain text" {
		t.Errorf("Render(%q) = %q, want unchanged", "just plain text", got)
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPrefix string
		wantRest   string
	}{
		{
			name:       "single shortcode with text",
			input:      ":docker: Build image",
			wantPrefix: ":docker:",
			wantRest:   "Build image",
		},
		{
			name:       "multiple shortcodes",
			input:      ":docker: :golang: Build",
			wantPrefix: ":docker: :golang:",
			wantRest:   "Build",
		},
		{
			name:       "no shortcodes",
			input:      "Build image",
			wantPrefix: "",
			wantRest:   "Build image",
		},
		{
			name:       "shortcode only",
			input:      ":pipeline:",
			wantPrefix: ":pipeline:",
			wantRest:   "",
		},
		{
			name:       "shortcode with hyphen",
			input:      ":golangci-lint: lint",
			wantPrefix: ":golangci-lint:",
			wantRest:   "lint",
		},
		{
			name:       "shortcode in middle not matched",
			input:      "Build :docker: image",
			wantPrefix: "",
			wantRest:   "Build :docker: image",
		},
		{
			name:       "empty string",
			input:      "",
			wantPrefix: "",
			wantRest:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, rest := Split(tt.input)
			if prefix != tt.wantPrefix || rest != tt.wantRest {
				t.Errorf("Split(%q) = (%q, %q), want (%q, %q)",
					tt.input, prefix, rest, tt.wantPrefix, tt.wantRest)
			}
		})
	}
}
