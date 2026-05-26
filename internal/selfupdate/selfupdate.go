package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxDownloadSize caps the size of a release archive we will fetch. The
// largest bk archive at time of writing is well under 50 MiB; 200 MiB
// gives plenty of headroom while preventing a runaway download. Declared
// as a var so tests can shrink it.
var maxDownloadSize int64 = 200 << 20

// httpClient is shared across release-fetching calls. ResponseHeaderTimeout
// guards against stalled servers without aborting long-but-progressing
// downloads.
var httpClient = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

type InstallMethod string

const (
	InstallMethodStandalone InstallMethod = "standalone"
	InstallMethodHomebrew   InstallMethod = "homebrew"
	InstallMethodMise       InstallMethod = "mise"
)

type Installation struct {
	Path         string
	ResolvedPath string
	Method       InstallMethod
	BrewFormula  string
}

var (
	executablePath         = os.Executable
	evalSymlinks           = filepath.EvalSymlinks
	releaseDownloadBaseURL = "https://github.com/buildkite/cli/releases/download"
)

func CurrentInstallation() (Installation, error) {
	path, err := executablePath()
	if err != nil {
		return Installation{}, err
	}

	resolved := path
	if realPath, err := evalSymlinks(path); err == nil {
		resolved = realPath
	}

	return DetectInstallation(path, resolved), nil
}

func DetectInstallation(path, resolvedPath string) Installation {
	installation := Installation{
		Path:         path,
		ResolvedPath: resolvedPath,
		Method:       InstallMethodStandalone,
	}

	for _, candidate := range []string{resolvedPath, path} {
		if candidate == "" {
			continue
		}
		normalized := strings.ToLower(filepath.ToSlash(candidate))

		if isHomebrewPath(normalized) {
			installation.Method = InstallMethodHomebrew
			installation.BrewFormula = brewFormula(candidate)
			return installation
		}
		if isMisePath(normalized) {
			installation.Method = InstallMethodMise
			return installation
		}
	}

	return installation
}

func (i Installation) TargetPath() string {
	if i.ResolvedPath != "" {
		return i.ResolvedPath
	}
	return i.Path
}

func UpdateInstruction(installation Installation) string {
	switch installation.Method {
	case InstallMethodHomebrew:
		formula := installation.BrewFormula
		if formula == "" {
			formula = "bk"
		}
		return fmt.Sprintf("brew upgrade %s", formula)
	case InstallMethodMise:
		return "update it with mise"
	default:
		return "bk update"
	}
}

func BuildDownloadURL(version, targetOS, targetArch string) string {
	return fmt.Sprintf("%s/v%s/%s", releaseDownloadBaseURL, version, ArchiveName(version, targetOS, targetArch))
}

func BuildChecksumURL(version string) string {
	return fmt.Sprintf("%s/v%s/bk_%s_checksums.txt", releaseDownloadBaseURL, version, version)
}

func ArchiveName(version, targetOS, targetArch string) string {
	osName := targetOS
	extension := "tar.gz"

	switch targetOS {
	case "darwin":
		osName = "macOS"
		extension = "zip"
	case "windows":
		osName = "windows"
		extension = "zip"
	}

	return fmt.Sprintf("bk_%s_%s_%s.%s", version, osName, targetArch, extension)
}

func BinaryName(targetOS string) string {
	if targetOS == "windows" {
		return "bk.exe"
	}
	return "bk"
}

func FetchExpectedSHA256(sumsURL, archiveFilename string) (string, error) {
	resp, err := httpClient.Get(sumsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching checksums failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) == 2 && parts[1] == archiveFilename {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no SHA256 checksum found for %s", archiveFilename)
}

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

func DownloadToTemp(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	if resp.ContentLength > maxDownloadSize {
		return "", fmt.Errorf("download size %d exceeds maximum of %d bytes", resp.ContentLength, maxDownloadSize)
	}

	tmpFile, err := os.CreateTemp("", "bk-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// LimitReader caps the bytes copied; the +1 lets us detect when the
	// payload exceeds the limit even if Content-Length was missing or wrong.
	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	if written > maxDownloadSize {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download exceeded maximum of %d bytes", maxDownloadSize)
	}

	return tmpFile.Name(), nil
}

func ReplaceBinary(archivePath, targetPath, targetOS string) error {
	if targetOS == "windows" {
		return fmt.Errorf("self-update is not supported on Windows yet; please download a new release manually")
	}

	workDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".bk-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	if err := ExtractBinary(archivePath, workDir, targetOS); err != nil {
		return err
	}

	newBinary := filepath.Join(workDir, BinaryName(targetOS))
	if err := os.Chmod(newBinary, 0o755); err != nil {
		return err
	}

	return os.Rename(newBinary, targetPath)
}

func ExtractBinary(archivePath, dest, targetOS string) error {
	if targetOS == "linux" {
		return extractTarGz(archivePath, dest, BinaryName(targetOS))
	}
	return extractZip(archivePath, dest, BinaryName(targetOS))
}

func extractTarGz(archivePath, dest, binaryName string) error {
	f, err := os.Open(archivePath)
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

		if filepath.Base(header.Name) != binaryName {
			continue
		}

		out, err := os.OpenFile(filepath.Join(dest, binaryName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("%s not found in archive", binaryName)
}

func extractZip(archivePath, dest, binaryName string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, file := range zr.File {
		if filepath.Base(file.Name) != binaryName {
			continue
		}

		in, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(filepath.Join(dest, binaryName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			in.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			in.Close()
			return err
		}
		if err := out.Close(); err != nil {
			in.Close()
			return err
		}
		if err := in.Close(); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("%s not found in archive", binaryName)
}

func isHomebrewPath(path string) bool {
	return strings.Contains(path, "/cellar/") ||
		strings.Contains(path, "/homebrew/") ||
		strings.Contains(path, "/.linuxbrew/")
}

func isMisePath(path string) bool {
	return strings.Contains(path, "/mise/shims/") ||
		strings.Contains(path, "/mise/installs/") ||
		strings.Contains(path, "/.local/share/mise/") ||
		strings.Contains(path, "/library/application support/mise/")
}

func brewFormula(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		if strings.EqualFold(part, "Cellar") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "bk"
}
