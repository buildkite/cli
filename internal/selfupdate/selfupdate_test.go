package selfupdate

import (
	"archive/tar"
	"archive/zip"
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
)

func TestDetectInstallation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		resolvedPath string
		wantMethod   InstallMethod
		wantFormula  string
	}{
		{
			name:         "homebrew via cellar",
			path:         "/opt/homebrew/bin/bk",
			resolvedPath: "/opt/homebrew/Cellar/bk@3/3.44.0/bin/bk",
			wantMethod:   InstallMethodHomebrew,
			wantFormula:  "bk@3",
		},
		{
			name:         "mise shim",
			path:         "/Users/ben/.local/share/mise/shims/bk",
			resolvedPath: "/Users/ben/.local/share/mise/installs/ubi-buildkite-cli/3.44.0/bin/bk",
			wantMethod:   InstallMethodMise,
		},
		{
			name:         "standalone binary",
			path:         "/Users/ben/bin/bk",
			resolvedPath: "/Users/ben/bin/bk",
			wantMethod:   InstallMethodStandalone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DetectInstallation(tt.path, tt.resolvedPath)
			if got.Method != tt.wantMethod {
				t.Fatalf("Method = %q, want %q", got.Method, tt.wantMethod)
			}
			if got.BrewFormula != tt.wantFormula {
				t.Fatalf("BrewFormula = %q, want %q", got.BrewFormula, tt.wantFormula)
			}
		})
	}
}

func TestBuildURLs(t *testing.T) {
	t.Parallel()

	if got, want := BuildDownloadURL("3.44.0", "linux", "amd64"), "https://github.com/buildkite/cli/releases/download/v3.44.0/bk_3.44.0_linux_amd64.tar.gz"; got != want {
		t.Fatalf("BuildDownloadURL(linux) = %q, want %q", got, want)
	}
	if got, want := BuildDownloadURL("3.44.0", "darwin", "arm64"), "https://github.com/buildkite/cli/releases/download/v3.44.0/bk_3.44.0_macOS_arm64.zip"; got != want {
		t.Fatalf("BuildDownloadURL(darwin) = %q, want %q", got, want)
	}
	if got, want := BuildChecksumURL("3.44.0"), "https://github.com/buildkite/cli/releases/download/v3.44.0/bk_3.44.0_checksums.txt"; got != want {
		t.Fatalf("BuildChecksumURL() = %q, want %q", got, want)
	}
}

func TestFetchExpectedSHA256(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc123  bk_3.44.0_linux_amd64.tar.gz\ndef456  bk_3.44.0_macOS_arm64.zip\n")
	}))
	defer server.Close()

	got, err := FetchExpectedSHA256(server.URL, "bk_3.44.0_macOS_arm64.zip")
	if err != nil {
		t.Fatalf("FetchExpectedSHA256() error = %v", err)
	}
	if got != "def456" {
		t.Fatalf("FetchExpectedSHA256() = %q, want def456", got)
	}
}

func TestVerifySHA256(t *testing.T) {
	t.Parallel()

	content := []byte("hello bk")
	hash := sha256.Sum256(content)
	path := filepath.Join(t.TempDir(), "bk")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifySHA256(path, hex.EncodeToString(hash[:])); err != nil {
		t.Fatalf("VerifySHA256() error = %v", err)
	}

	if err := VerifySHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("VerifySHA256() succeeded with wrong hash")
	}
}

func TestDownloadToTemp_RejectsOversizedPayload(t *testing.T) {
	originalMax := maxDownloadSize
	maxDownloadSize = 16
	t.Cleanup(func() { maxDownloadSize = originalMax })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Content-Length so we exercise the LimitReader path.
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("A", 64)))
	}))
	defer server.Close()

	path, err := DownloadToTemp(server.URL)
	if err == nil {
		os.Remove(path)
		t.Fatal("DownloadToTemp() succeeded, want size error")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("expected size error, got %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("temp file %q was not cleaned up after rejection", path)
	}
}

func TestExtractBinary(t *testing.T) {
	t.Parallel()

	t.Run("extracts linux tar.gz", func(t *testing.T) {
		t.Parallel()
		archive := filepath.Join(t.TempDir(), "bk.tar.gz")
		createTarGz(t, archive, map[string]string{
			"bk_3.44.0_linux_amd64/README.md": "ignore",
			"bk_3.44.0_linux_amd64/bk":        "linux binary",
		})

		dest := t.TempDir()
		if err := ExtractBinary(archive, dest, "linux"); err != nil {
			t.Fatalf("ExtractBinary() error = %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "bk"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "linux binary" {
			t.Fatalf("extracted content = %q", got)
		}
	})

	t.Run("extracts macOS zip", func(t *testing.T) {
		t.Parallel()
		archive := filepath.Join(t.TempDir(), "bk.zip")
		createZip(t, archive, map[string]string{
			"bk_3.44.0_macOS_arm64/README.md": "ignore",
			"bk_3.44.0_macOS_arm64/bk":        "darwin binary",
		})

		dest := t.TempDir()
		if err := ExtractBinary(archive, dest, "darwin"); err != nil {
			t.Fatalf("ExtractBinary() error = %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "bk"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "darwin binary" {
			t.Fatalf("extracted content = %q", got)
		}
	})
}

func createTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o755}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
}

func createZip(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
}
