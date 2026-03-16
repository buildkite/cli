package agent

import (
	"strings"
	"testing"
)

func TestDefaultBinDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		os       string
		contains string
	}{
		{"linux uses .buildkite-agent", "linux", ".buildkite-agent/bin"},
		{"darwin uses .buildkite-agent", "darwin", ".buildkite-agent/bin"},
		{"windows uses buildkite", "windows", "buildkite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DefaultBinDir(tt.os)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("DefaultBinDir(%q) = %q, expected to contain %q", tt.os, got, tt.contains)
			}
		})
	}
}

func TestDefaultBuildPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		os       string
		contains string
	}{
		{"linux uses .buildkite-agent", "linux", ".buildkite-agent/builds"},
		{"darwin uses .buildkite-agent", "darwin", ".buildkite-agent/builds"},
		{"windows uses buildkite", "windows", "buildkite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DefaultBuildPath(tt.os)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("DefaultBuildPath(%q) = %q, expected to contain %q", tt.os, got, tt.contains)
			}
		})
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		os       string
		contains string
	}{
		{"linux uses .buildkite-agent", "linux", ".buildkite-agent/buildkite-agent.cfg"},
		{"darwin uses .buildkite-agent", "darwin", ".buildkite-agent/buildkite-agent.cfg"},
		{"windows uses buildkite", "windows", "buildkite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DefaultConfigPath(tt.os)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("DefaultConfigPath(%q) = %q, expected to contain %q", tt.os, got, tt.contains)
			}
		})
	}
}
