package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/selfupdate"
)

func TestUpdateCmd_RunStandaloneSelfUpdates(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "bk")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	archiveData := makeTarGz(t, map[string]string{
		"bk_3.2.0_linux_amd64/README.md": "ignore",
		"bk_3.2.0_linux_amd64/bk":        "new binary",
	})
	hash := sha256.Sum256(archiveData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bk_3.2.0_linux_amd64.tar.gz":
			_, _ = w.Write(archiveData)
		case "/bk_3.2.0_checksums.txt":
			fmt.Fprintf(w, "%s  bk_3.2.0_linux_amd64.tar.gz\n", hex.EncodeToString(hash[:]))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	cmd := &UpdateCmd{
		stdout:     &output,
		version:    "3.1.0",
		targetOS:   "linux",
		targetArch: "amd64",
		currentInstallation: func() (selfupdate.Installation, error) {
			return selfupdate.Installation{Path: target, ResolvedPath: target, Method: selfupdate.InstallMethodStandalone}, nil
		},
		latestReleaseVersion: func() (string, error) { return "3.2.0", nil },
		buildDownloadURL:     func(string, string, string) string { return server.URL + "/bk_3.2.0_linux_amd64.tar.gz" },
		buildChecksumURL:     func(string) string { return server.URL + "/bk_3.2.0_checksums.txt" },
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("updated binary = %q, want %q", got, "new binary")
	}
	if !strings.Contains(output.String(), "Updated bk to version 3.2.0") {
		t.Fatalf("expected success output, got %q", output.String())
	}
}

func TestUpdateCmd_RunHomebrewPrintsInstructions(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	cmd := &UpdateCmd{
		stdout:  &output,
		version: "3.44.0",
		currentInstallation: func() (selfupdate.Installation, error) {
			return selfupdate.Installation{
				Path:         "/opt/homebrew/bin/bk",
				ResolvedPath: "/opt/homebrew/Cellar/bk@3/3.44.0/bin/bk",
				Method:       selfupdate.InstallMethodHomebrew,
				BrewFormula:  "bk@3",
			}, nil
		},
		latestReleaseVersion: func() (string, error) { return "3.45.0", nil },
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "managed by Homebrew") {
		t.Fatalf("expected Homebrew output, got %q", got)
	}
	if !strings.Contains(got, "brew upgrade bk@3") {
		t.Fatalf("expected brew instruction, got %q", got)
	}
}

func TestUpdateCmd_RunHomebrewWarnsOnLatestReleaseError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	cmd := &UpdateCmd{
		stdout:  &stdout,
		stderr:  &stderr,
		version: "3.44.0",
		currentInstallation: func() (selfupdate.Installation, error) {
			return selfupdate.Installation{
				Path:         "/opt/homebrew/bin/bk",
				ResolvedPath: "/opt/homebrew/Cellar/bk@3/3.44.0/bin/bk",
				Method:       selfupdate.InstallMethodHomebrew,
				BrewFormula:  "bk@3",
			}, nil
		},
		latestReleaseVersion: func() (string, error) { return "", fmt.Errorf("network down") },
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(stderr.String(), "could not check for the latest release") {
		t.Fatalf("expected warning on stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "brew upgrade bk@3") {
		t.Fatalf("expected brew instruction on stdout, got %q", stdout.String())
	}
}

func TestUpdateCmd_RunStandaloneDevBuildRefusesSelfUpdate(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	cmd := &UpdateCmd{
		stdout:  &output,
		version: "DEV",
		currentInstallation: func() (selfupdate.Installation, error) {
			return selfupdate.Installation{Path: "/tmp/bk", ResolvedPath: "/tmp/bk", Method: selfupdate.InstallMethodStandalone}, nil
		},
		latestReleaseVersion: func() (string, error) {
			t.Fatal("latestReleaseVersion should not be called for dev builds")
			return "", nil
		},
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "released builds") {
		t.Fatalf("expected released-builds error, got %v", err)
	}
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o755}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}
