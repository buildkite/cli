package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/internal/selfupdate"
)

var releaseURL = "https://api.github.com/repos/buildkite/cli/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// LatestReleaseVersion returns the latest released bk version from GitHub.
func LatestReleaseVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// CheckForUpdate checks GitHub for the latest release and returns the latest
// version string and whether it is newer than currentVersion.
// Returns ("", false) silently on any error or if no update is available.
func CheckForUpdate(currentVersion string) (string, bool) {
	current := strings.TrimPrefix(currentVersion, "v")
	if !IsReleaseVersion(current) {
		return "", false
	}

	latest, err := LatestReleaseVersion()
	if err != nil {
		return "", false
	}

	if HasUpdate(current, latest) {
		return latest, true
	}

	return "", false
}

// HasUpdate returns true if latestVersion is strictly newer than currentVersion.
func HasUpdate(currentVersion, latestVersion string) bool {
	return isNewer(strings.TrimPrefix(latestVersion, "v"), strings.TrimPrefix(currentVersion, "v"))
}

// IsReleaseVersion reports whether v is a plain major.minor.patch release.
func IsReleaseVersion(v string) bool {
	return parseVersion(strings.TrimPrefix(v, "v")) != nil
}

// isNewer returns true if version a is strictly newer than version b.
// Both versions are expected to be in "major.minor.patch" format.
func isNewer(a, b string) bool {
	aParts := parseVersion(a)
	bParts := parseVersion(b)
	if aParts == nil || bParts == nil {
		return false
	}

	for i := range 3 {
		if aParts[i] > bParts[i] {
			return true
		}
		if aParts[i] < bParts[i] {
			return false
		}
	}
	return false
}

// parseVersion splits a "major.minor.patch" string into three integers.
// Returns nil if the format is invalid.
func parseVersion(v string) []int {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}

	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// FormatUpdateNudge returns the nudge message for display.
func FormatUpdateNudge(latestVersion string, installation selfupdate.Installation) string {
	message := fmt.Sprintf("A new version of bk is available: %s\n", latestVersion)

	switch installation.Method {
	case selfupdate.InstallMethodHomebrew:
		return message + fmt.Sprintf("Update it with Homebrew: %s\n", selfupdate.UpdateInstruction(installation))
	case selfupdate.InstallMethodMise:
		return message + "Update it with mise.\n"
	default:
		return message + "Run 'bk update' to update.\n"
	}
}
