package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
)

func validateConfig(conf *config.Config) error {
	if conf.APIToken() == "" {
		return bkErrors.NewConfigurationError(nil, "API token not configured. run `bk configure` to set it up")
	}
	if conf.OrganizationSlug() == "" {
		return fmt.Errorf("no organization selected. run `bk use` to select one")
	}
	return nil
}

// parseBuildIdentifier parses a build identifier which can be:
// - A build URL (e.g., "https://buildkite.com/org/pipeline/builds/123")
// - An org/pipeline/number format (e.g., "my-org/my-pipeline/123")
// - A pipeline/number format (e.g., "my-pipeline/123")
// - A build number (e.g., "123") - will need pipeline context
func parseBuildIdentifier(identifier, defaultOrg string) (org, pipeline, buildNumber string, err error) {
	// If it looks like a URL, parse it
	if strings.HasPrefix(identifier, "http") {
		u, parseErr := url.Parse(identifier)
		if parseErr != nil {
			return "", "", "", fmt.Errorf("invalid build URL: %w", parseErr)
		}

		// Expected format: /org/pipeline/builds/number
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 4 && parts[2] == "builds" {
			return parts[0], parts[1], parts[3], nil
		}
		return "", "", "", fmt.Errorf("invalid build URL format")
	}

	// Check for org/pipeline/number or pipeline/number format
	if strings.Contains(identifier, "/") {
		parts := strings.Split(identifier, "/")
		if len(parts) >= 3 {
			// org/pipeline/number format
			return parts[0], parts[1], parts[2], nil
		}
		if len(parts) == 2 {
			// pipeline/number format - use default org
			return defaultOrg, parts[0], parts[1], nil
		}
		return "", "", "", fmt.Errorf("invalid format")
	}

	// Just a build number - return empty org/pipeline
	return "", "", identifier, nil
}
