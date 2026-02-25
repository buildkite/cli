package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
)

// CommandsNotRequiringToken is a list of command paths that don't require an API token
var CommandsNotRequiringToken = []string{
	"pipeline validate", // The pipeline validate command doesn't require an API token
	"pipeline migrate",  // The pipeline migrate command uses a public migration API
}

// ValidateConfiguration checks that the configuration is valid to execute the command (Kong version)
func ValidateConfiguration(conf *config.Config, commandPath string) error {
	missingToken := conf.APIToken() == ""
	missingOrg := conf.OrganizationSlug() == ""

	// Skip token check for all configure commands
	if strings.HasPrefix(commandPath, "configure") {
		return nil
	}

	// Skip token check for commands that don't need it
	for _, exemptCmd := range CommandsNotRequiringToken {
		// Check if the command path ends with the exempt command pattern
		if strings.HasSuffix(commandPath, exemptCmd) {
			return nil // Skip validation for exempt commands
		}
	}

	switch {
	case missingToken && missingOrg:
		return errors.New("you are not authenticated. Run bk auth login to authenticate, or run bk use to select a configured organization")
	case missingToken:
		return errors.New("you are not authenticated. Run bk auth login to authenticate")
	// an organization may not be present if the user is only viewing public resources
	case missingOrg:
		fmt.Println("Warning: no organization set, only public pipelines will be visible. Run bk auth login, or bk use, to set an organization")
		return nil
	}

	return nil
}
