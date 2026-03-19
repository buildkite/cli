package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExistingInstall describes a buildkite-agent binary already present on the system.
type ExistingInstall struct {
	Path    string
	Version string
}

// FindExisting looks for buildkite-agent in PATH and returns info about it.
// Returns nil if no existing installation is found.
func FindExisting(targetOS string) *ExistingInstall {
	name := BinaryName(targetOS)
	path, err := exec.LookPath(name)
	if err != nil {
		return nil
	}

	install := &ExistingInstall{Path: path}

	out, err := exec.Command(path, "--version").Output()
	if err == nil {
		version := strings.TrimSpace(string(out))
		// Output is like "buildkite-agent version 3.119.2+11755.abc123..."
		// Extract just the semver portion.
		version = strings.TrimPrefix(version, "buildkite-agent version ")
		if plusIdx := strings.Index(version, "+"); plusIdx != -1 {
			version = version[:plusIdx]
		}
		install.Version = version
	}

	return install
}

// ResolveLatestVersion queries the GitHub API for the latest buildkite-agent release tag.
func ResolveLatestVersion() (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/buildkite/agent/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// BuildDownloadURL returns the GitHub releases URL for the given agent version, OS, and arch.
func BuildDownloadURL(version, os, arch string) string {
	var extension string
	switch os {
	case "windows":
		extension = "zip"
	default:
		extension = "tar.gz"
	}

	return fmt.Sprintf(
		"https://github.com/buildkite/agent/releases/download/v%s/buildkite-agent-%s-%s-%s.%s",
		version, os, arch, version, extension,
	)
}

// BuildSHA256SumsURL returns the URL for the SHA256SUMS file for a given agent version.
func BuildSHA256SumsURL(version string) string {
	return fmt.Sprintf(
		"https://github.com/buildkite/agent/releases/download/v%s/buildkite-agent-%s.SHA256SUMS",
		version, version,
	)
}

// FetchExpectedSHA256 downloads the SHA256SUMS file and returns the expected hash
// for the given archive filename.
func FetchExpectedSHA256(sumsURL, archiveFilename string) (string, error) {
	resp, err := http.Get(sumsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching SHA256SUMS failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		// Format: "<hash>  <filename>"
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) == 2 && parts[1] == archiveFilename {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no SHA256 checksum found for %s", archiveFilename)
}

// VerifySHA256 computes the SHA256 hash of the file at path and compares it
// to the expected hex-encoded hash. Returns an error if they don't match.
func VerifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// DownloadToTemp downloads the given URL to a temporary file and returns its path.
// The caller is responsible for removing the file when done.
func DownloadToTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "buildkite-agent-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// ExtractBinary extracts the buildkite-agent binary from the given archive to dest.
func ExtractBinary(archive, dest, targetOS string) error {
	if targetOS == "windows" {
		return extractZip(archive, dest)
	}
	return extractTarGz(archive, dest)
}

// BinaryName returns the platform-appropriate binary name.
func BinaryName(targetOS string) string {
	if targetOS == "windows" {
		return "buildkite-agent.exe"
	}
	return "buildkite-agent"
}

func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if filepath.Base(header.Name) != "buildkite-agent" {
			continue
		}

		outPath := filepath.Join(dest, "buildkite-agent")
		out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, tr); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("buildkite-agent binary not found in archive")
}

func extractZip(archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) != "buildkite-agent.exe" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		outPath := filepath.Join(dest, "buildkite-agent.exe")
		out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, rc); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("buildkite-agent.exe not found in archive")
}
