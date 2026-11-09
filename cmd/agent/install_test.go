package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/cli"
)

func TestCheckConfigPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existing := filepath.Join(dir, "buildkite-agent.cfg")
	if err := os.WriteFile(existing, []byte("tags=\"queue=prod\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.cfg")
	underFile := filepath.Join(existing, "x")

	if err := checkConfigPath(missing, false); err != nil {
		t.Errorf("missing config: unexpected error %v", err)
	}
	if err := checkConfigPath(existing, true); err != nil {
		t.Errorf("existing config with force: unexpected error %v", err)
	}
	err := checkConfigPath(existing, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("existing config without force: expected --force error, got %v", err)
	}
	if err := checkConfigPath(dir, true); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("directory with force: expected directory error, got %v", err)
	}
	if err := checkConfigPath(underFile, false); err == nil || !strings.Contains(err.Error(), "checking agent config") {
		t.Errorf("path under a file: expected stat error, got %v", err)
	}
}

func TestInstallRefusesExistingConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "buildkite-agent.cfg")
	original := []byte("token=\"old\"\ntags=\"queue=prod\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	// The refusal must happen before anything is downloaded or installed to Dest.
	// The version deliberately does not exist, so a missing guard fails on download, not by minting a token.
	cmd := &InstallCmd{Version: "0.0.0-test", Dest: filepath.Join(t.TempDir(), "bin"), ConfigPath: path}
	if err := cmd.Run(nil, cli.Globals{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected refusal, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("config was modified:\n%s", got)
	}
	if _, err := os.Stat(cmd.Dest); !os.IsNotExist(err) {
		t.Errorf("expected nothing installed to %s before refusal", cmd.Dest)
	}
}
