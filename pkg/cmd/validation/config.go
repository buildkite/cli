package validation

import (
	"errors"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
)

// CommandsNotRequiringToken is a list of command paths that don't require an API token
var CommandsNotRequiringToken = []string{
	"pipeline validate", // The pipeline validate command doesn't require an API token
	"pipeline migrate",  // The pipeline migrate command uses a public migration API
}

// CheckValidConfiguration checks that the viper configuration is valid to execute the command (Kong version)
func ValidateConfiguration(conf *config.Config, commandPath string) error {
	missingToken := conf.APIToken() == ""
	missingOrg := conf.OrganizationSlug() == ""

	// Skip token check for commands that don't need it
	for _, exemptCmd := range CommandsNotRequiringToken {
		// Check if the command path ends with the exempt command pattern
		if strings.HasSuffix(commandPath, exemptCmd) {
			return nil // Skip validation for exempt commands
		}
	}

	switch {
	case missingToken && missingOrg:
		return errors.New("you must set a valid API token and organization slug. Run `bk configure` or `bk use`, or set the environment variables `BUILDKITE_API_TOKEN` and `BUILDKITE_ORGANIZATION_SLUG`")
	case missingToken:
		return errors.New("you must set a valid API token. Run `bk configure`, or set the environment variable `BUILDKITE_API_TOKEN`")
	case missingOrg:
		return errors.New("you must set a valid organization slug. Run `bk use`, or set the environment variable `BUILDKITE_ORGANIZATION_SLUG`")
	}

	return nil
}
