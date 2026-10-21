package agent

import (
	"os"
	"path/filepath"
	"strings"
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
			return LinuxSystemBuildPath
		}
		return filepath.Join(home, ".buildkite-agent", "builds")
	}
}

// LinuxSystemBuildPath is the build path the Linux packages configure for the
// buildkite-agent system service.
const LinuxSystemBuildPath = "/var/lib/buildkite-agent/builds"

// IsSystemConfigPath reports whether configPath is where the Linux packages'
// buildkite-agent service reads its config. That service runs as the
// buildkite-agent user, so it can't use a build path in the invoking user's home.
func IsSystemConfigPath(targetOS, configPath string) bool {
	if targetOS != "linux" {
		return false
	}
	return strings.HasPrefix(filepath.Clean(configPath), "/etc/buildkite-agent/")
}

// ResolveBuildPath returns buildPath if set, otherwise the default build path to
// pair with a config written to configPath.
func ResolveBuildPath(targetOS, configPath, buildPath string) string {
	if buildPath != "" {
		return buildPath
	}
	if IsSystemConfigPath(targetOS, configPath) {
		return LinuxSystemBuildPath
	}
	return DefaultBuildPath(targetOS)
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
