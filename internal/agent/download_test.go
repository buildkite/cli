package agent

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

func TestBuildDownloadURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		os      string
		arch    string
		want    string
	}{
		{
			name:    "linux amd64",
			version: "3.120.0",
			os:      "linux",
			arch:    "amd64",
			want:    "https://github.com/buildkite/agent/releases/download/v3.120.0/buildkite-agent-linux-amd64-3.120.0.tar.gz",
		},
		{
			name:    "darwin arm64",
			version: "3.120.0",
			os:      "darwin",
			arch:    "arm64",
			want:    "https://github.com/buildkite/agent/releases/download/v3.120.0/buildkite-agent-darwin-arm64-3.120.0.tar.gz",
		},
		{
			name:    "windows amd64",
			version: "3.120.0",
			os:      "windows",
			arch:    "amd64",
			want:    "https://github.com/buildkite/agent/releases/download/v3.120.0/buildkite-agent-windows-amd64-3.120.0.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BuildDownloadURL(tt.version, tt.os, tt.arch)
			if got != tt.want {
				t.Errorf("BuildDownloadURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSHA256SumsURL(t *testing.T) {
	t.Parallel()

	got := BuildSHA256SumsURL("3.120.0")
	want := "https://github.com/buildkite/agent/releases/download/v3.120.0/buildkite-agent-3.120.0.SHA256SUMS"
	if got != want {
		t.Errorf("BuildSHA256SumsURL() = %q, want %q", got, want)
	}
}

func TestFetchExpectedSHA256(t *testing.T) {
	t.Parallel()

	sumsBody := "abc123  buildkite-agent-linux-amd64-3.120.0.tar.gz\ndef456  buildkite-agent-darwin-arm64-3.120.0.tar.gz\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sumsBody)
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name     string
		filename string
		want     string
		wantErr  bool
	}{
		{"found linux", "buildkite-agent-linux-amd64-3.120.0.tar.gz", "abc123", false},
		{"found darwin", "buildkite-agent-darwin-arm64-3.120.0.tar.gz", "def456", false},
		{"not found", "buildkite-agent-windows-amd64-3.120.0.zip", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := FetchExpectedSHA256(server.URL, tt.filename)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("FetchExpectedSHA256() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	t.Parallel()

	content := []byte("hello buildkite agent")
	hash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(hash[:])

	tmpFile := filepath.Join(t.TempDir(), "testfile")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Should pass with correct hash
	if err := VerifySHA256(tmpFile, expectedHex); err != nil {
		t.Errorf("VerifySHA256() with correct hash: unexpected error: %v", err)
	}

	// Should fail with wrong hash
	err := VerifySHA256(tmpFile, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("VerifySHA256() with wrong hash: expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "SHA256 mismatch") {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestBinaryName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		os   string
		want string
	}{
		{"linux", "buildkite-agent"},
		{"darwin", "buildkite-agent"},
		{"windows", "buildkite-agent.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			t.Parallel()
			got := BinaryName(tt.os)
			if got != tt.want {
				t.Errorf("BinaryName(%q) = %q, want %q", tt.os, got, tt.want)
			}
		})
	}
}

func TestExtractTarGz(t *testing.T) {
	t.Parallel()

	// Create a tar.gz archive containing a fake buildkite-agent binary
	archivePath := filepath.Join(t.TempDir(), "agent.tar.gz")
	binaryContent := []byte("#!/bin/sh\necho hello\n")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a non-matching file first
	if err := tw.WriteHeader(&tar.Header{Name: "README.md", Size: 5, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	// Add the buildkite-agent binary
	if err := tw.WriteHeader(&tar.Header{Name: "buildkite-agent", Size: int64(len(binaryContent)), Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	// Extract to a temp dir
	dest := t.TempDir()
	if err := extractTarGz(archivePath, dest); err != nil {
		t.Fatalf("extractTarGz() error: %v", err)
	}

	// Verify the binary was extracted
	extracted, err := os.ReadFile(filepath.Join(dest, "buildkite-agent"))
	if err != nil {
		t.Fatalf("reading extracted binary: %v", err)
	}
	if string(extracted) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", extracted, binaryContent)
	}

	// Verify README.md was NOT extracted
	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Error("README.md should not have been extracted")
	}
}

func TestExtractTarGz_MissingBinary(t *testing.T) {
	t.Parallel()

	// Create a tar.gz with no buildkite-agent file
	archivePath := filepath.Join(t.TempDir(), "agent.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	if err := tw.WriteHeader(&tar.Header{Name: "other-file", Size: 5, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	dest := t.TempDir()
	err = extractTarGz(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if err.Error() != "buildkite-agent binary not found in archive" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractZip(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "agent.zip")
	binaryContent := []byte("fake-exe-content")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	// Add a non-matching file
	w, err := zw.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	// Add the binary
	w, err = zw.Create("buildkite-agent.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(binaryContent); err != nil {
		t.Fatal(err)
	}

	zw.Close()
	f.Close()

	dest := t.TempDir()
	if err := extractZip(archivePath, dest); err != nil {
		t.Fatalf("extractZip() error: %v", err)
	}

	extracted, err := os.ReadFile(filepath.Join(dest, "buildkite-agent.exe"))
	if err != nil {
		t.Fatalf("reading extracted binary: %v", err)
	}
	if string(extracted) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", extracted, binaryContent)
	}

	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Error("README.md should not have been extracted")
	}
}

func TestExtractZip_MissingBinary(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "agent.zip")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)
	w, err := zw.Create("other-file")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	zw.Close()
	f.Close()

	dest := t.TempDir()
	err = extractZip(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if err.Error() != "buildkite-agent.exe not found in archive" {
		t.Errorf("unexpected error: %v", err)
	}
}
