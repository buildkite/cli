package agent

import (
	"os"
	"path/filepath"
)

// DefaultBinDir returns the platform-appropriate default directory for the agent binary.
func DefaultBinDir(targetOS string) string {
	switch targetOS {
	case "windows":
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			return filepath.Join(appData, "Buildkite", "bin")
		}
		return filepath.Join("C:\\", "Program Files", "buildkite", "bin")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "/usr/local/bin"
		}
		return filepath.Join(home, ".buildkite-agent", "bin")
	}
}

// DefaultBuildPath returns the platform-appropriate default directory for agent builds.
func DefaultBuildPath(targetOS string) string {
	switch targetOS {
	case "windows":
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			return filepath.Join(appData, "Buildkite", "builds")
		}
		return filepath.Join("C:\\", "Program Files", "buildkite", "builds")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "/var/lib/buildkite-agent/builds"
		}
		return filepath.Join(home, ".buildkite-agent", "builds")
	}
}

// DefaultConfigPath returns the platform-appropriate default path for the agent config file.
func DefaultConfigPath(targetOS string) string {
	switch targetOS {
	case "windows":
		if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
			return filepath.Join(appData, "Buildkite", "buildkite-agent.cfg")
		}
		return filepath.Join("C:\\", "Program Files", "buildkite", "buildkite-agent.cfg")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "/etc/buildkite-agent/buildkite-agent.cfg"
		}
		return filepath.Join(home, ".buildkite-agent", "buildkite-agent.cfg")
	}
}
