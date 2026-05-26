package job

import (
	"bytes"
	"strings"
	"testing"
)

func TestWarnIgnoredJobContextFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pipeline    string
		buildNumber string
		wantWarning bool
	}{
		{
			name:        "no flags",
			wantWarning: false,
		},
		{
			name:        "pipeline",
			pipeline:    "cli",
			wantWarning: true,
		},
		{
			name:        "build",
			buildNumber: "123",
			wantWarning: true,
		},
		{
			name:        "both flags",
			pipeline:    "cli",
			buildNumber: "123",
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			warnIgnoredJobContextFlags(&stderr, tt.pipeline, tt.buildNumber)

			got := stderr.String()
			if tt.wantWarning {
				if !strings.Contains(got, "Warning: --pipeline and --build are deprecated and ignored") {
					t.Fatalf("warning = %q", got)
				}
				return
			}
			if got != "" {
				t.Fatalf("warning = %q, want empty", got)
			}
		})
	}
}
