package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	versionPkg "github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/internal/selfupdate"
)

// UpdateCmd updates the installed bk CLI in place, or prints the right
// instruction when bk is managed by Homebrew or mise.
//
// The unexported fields are dependency-injection seams for tests. Kong
// constructs the command with all fields zero-valued; Run() fills in the
// real implementations.
type UpdateCmd struct {
	stdout               io.Writer
	stderr               io.Writer
	version              string
	targetOS             string
	targetArch           string
	currentInstallation  func() (selfupdate.Installation, error)
	latestReleaseVersion func() (string, error)
	buildDownloadURL     func(version, targetOS, targetArch string) string
	buildChecksumURL     func(version string) string
}

func (c *UpdateCmd) Help() string {
	return `Update the installed bk CLI.

If bk is managed by Homebrew or mise, this command prints the right update
instruction for that tool.

If bk was installed as a standalone release binary, this command downloads the
latest release for the current platform, verifies its checksum, and replaces
that binary in place.
`
}

func (c *UpdateCmd) Run() error {
	c.applyDefaults()

	installation, err := c.currentInstallation()
	if err != nil {
		return fmt.Errorf("determining current installation: %w", err)
	}

	current := strings.TrimPrefix(c.version, "v")

	switch installation.Method {
	case selfupdate.InstallMethodHomebrew, selfupdate.InstallMethodMise:
		return c.printManagedInstallMessage(installation, current)
	}

	if !versionPkg.IsReleaseVersion(current) {
		return fmt.Errorf("self-update is only supported for released builds (current version: %s)", c.version)
	}

	latest, err := c.latestReleaseVersion()
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}
	if !versionPkg.HasUpdate(current, latest) {
		fmt.Fprintf(c.stdout, "bk is already up to date (%s)\n", current)
		return nil
	}

	if c.targetOS == "windows" {
		return fmt.Errorf("self-update is not supported on Windows yet; please download a new release manually")
	}

	fmt.Fprintf(c.stdout, "Downloading bk %s for %s/%s...\n", latest, c.targetOS, c.targetArch)
	downloadURL := c.buildDownloadURL(latest, c.targetOS, c.targetArch)
	archivePath, err := selfupdate.DownloadToTemp(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading bk: %w", err)
	}
	defer os.Remove(archivePath)

	fmt.Fprintln(c.stdout, "Verifying checksum...")
	expectedHash, err := selfupdate.FetchExpectedSHA256(c.buildChecksumURL(latest), filepath.Base(downloadURL))
	if err != nil {
		return fmt.Errorf("fetching checksum: %w", err)
	}
	if err := selfupdate.VerifySHA256(archivePath, expectedHash); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := selfupdate.ReplaceBinary(archivePath, installation.TargetPath(), c.targetOS); err != nil {
		return fmt.Errorf("installing updated bk: %w", err)
	}

	fmt.Fprintf(c.stdout, "Updated bk to version %s\n", latest)
	return nil
}

func (c *UpdateCmd) printManagedInstallMessage(installation selfupdate.Installation, current string) error {
	latest, err := c.latestReleaseVersion()
	switch {
	case err != nil:
		fmt.Fprintf(c.stderr, "Warning: could not check for the latest release: %v\n", err)
	case versionPkg.IsReleaseVersion(current) && !versionPkg.HasUpdate(current, latest):
		fmt.Fprintf(c.stdout, "bk is already up to date (%s)\n", current)
		return nil
	default:
		fmt.Fprintf(c.stdout, "A new version of bk is available: %s\n", latest)
	}

	switch installation.Method {
	case selfupdate.InstallMethodHomebrew:
		fmt.Fprintln(c.stdout, "This installation is managed by Homebrew.")
		fmt.Fprintf(c.stdout, "Update it with: %s\n", selfupdate.UpdateInstruction(installation))
	case selfupdate.InstallMethodMise:
		fmt.Fprintln(c.stdout, "This installation is managed by mise.")
		fmt.Fprintln(c.stdout, "Update it with mise.")
	}

	return nil
}

func (c *UpdateCmd) applyDefaults() {
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.stderr == nil {
		c.stderr = os.Stderr
	}
	if c.version == "" {
		c.version = versionPkg.Version
	}
	if c.targetOS == "" {
		c.targetOS = runtime.GOOS
	}
	if c.targetArch == "" {
		c.targetArch = runtime.GOARCH
	}
	if c.currentInstallation == nil {
		c.currentInstallation = selfupdate.CurrentInstallation
	}
	if c.latestReleaseVersion == nil {
		c.latestReleaseVersion = versionPkg.LatestReleaseVersion
	}
	if c.buildDownloadURL == nil {
		c.buildDownloadURL = selfupdate.BuildDownloadURL
	}
	if c.buildChecksumURL == nil {
		c.buildChecksumURL = selfupdate.BuildChecksumURL
	}
}
