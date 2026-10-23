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

func TestIsSystemConfigPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		os         string
		configPath string
		want       bool
	}{
		{"linux package config", "linux", "/etc/buildkite-agent/buildkite-agent.cfg", true},
		{"linux unclean package config", "linux", "/etc//buildkite-agent/./buildkite-agent.cfg", true},
		{"linux home config", "linux", "/root/.buildkite-agent/buildkite-agent.cfg", false},
		{"linux similar prefix", "linux", "/etc/buildkite-agent-old/buildkite-agent.cfg", false},
		{"linux escapes package dir", "linux", "/etc/buildkite-agent/../buildkite-agent.cfg", false},
		{"linux package dir itself", "linux", "/etc/buildkite-agent", false},
		{"darwin ignores package path", "darwin", "/etc/buildkite-agent/buildkite-agent.cfg", false},
		{"windows ignores package path", "windows", "/etc/buildkite-agent/buildkite-agent.cfg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSystemConfigPath(tt.os, tt.configPath); got != tt.want {
				t.Errorf("IsSystemConfigPath(%q, %q) = %v, want %v", tt.os, tt.configPath, got, tt.want)
			}
		})
	}
}

func TestResolveBuildPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		os         string
		configPath string
		buildPath  string
		want       string
	}{
		{"explicit wins over system config", "linux", "/etc/buildkite-agent/buildkite-agent.cfg", "/data/builds", "/data/builds"},
		{"explicit wins over home config", "linux", "/home/dev/.buildkite-agent/buildkite-agent.cfg", "/data/builds", "/data/builds"},
		{"linux system config uses package path", "linux", "/etc/buildkite-agent/buildkite-agent.cfg", "", LinuxSystemBuildPath},
		{"linux home config uses default", "linux", "/home/dev/.buildkite-agent/buildkite-agent.cfg", "", DefaultBuildPath("linux")},
		{"darwin ignores package path", "darwin", "/etc/buildkite-agent/buildkite-agent.cfg", "", DefaultBuildPath("darwin")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveBuildPath(tt.os, tt.configPath, tt.buildPath); got != tt.want {
				t.Errorf("ResolveBuildPath(%q, %q, %q) = %q, want %q", tt.os, tt.configPath, tt.buildPath, got, tt.want)
			}
		})
	}
}
