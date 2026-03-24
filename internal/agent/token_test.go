package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAgentConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "subdir", "buildkite-agent.cfg")

	err := WriteAgentConfig(configPath, "test-token-123", "/tmp/builds", []string{"queue=default"})
	if err != nil {
		t.Fatalf("WriteAgentConfig() error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	expected := "token=\"test-token-123\"\nbuild-path=\"/tmp/builds\"\ntags=\"queue=default\"\n"
	if string(content) != expected {
		t.Errorf("config content = %q, want %q", content, expected)
	}

	// Verify file permissions are restrictive
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config permissions = %o, want 600", perm)
	}
}

func TestWriteAgentConfig_CreatesParentDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "a", "b", "c", "buildkite-agent.cfg")

	err := WriteAgentConfig(configPath, "token", "/builds", nil)
	if err != nil {
		t.Fatalf("WriteAgentConfig() error: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}
