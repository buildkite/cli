package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var releaseURL = "https://api.github.com/repos/buildkite/cli/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdate checks GitHub for the latest release and returns the latest
// version string and whether it is newer than currentVersion.
// Returns ("", false) silently on any error or if no update is available.
func CheckForUpdate(currentVersion string) (string, bool) {
	current := strings.TrimPrefix(currentVersion, "v")
	if parseVersion(current) == nil {
		return "", false
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", false
	}

	latest := strings.TrimPrefix(release.TagName, "v")

	if isNewer(latest, current) {
		return latest, true
	}

	return "", false
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
func FormatUpdateNudge(latestVersion string) string {
	return fmt.Sprintf("A new version of bk is available: %s\n", latestVersion)
}
