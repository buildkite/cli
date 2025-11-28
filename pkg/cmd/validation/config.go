package validation

import (
	"errors"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/cobra"
)

// CommandsNotRequiringToken is a list of command paths that don't require an API token
var CommandsNotRequiringToken = []string{
	"pipeline validate", // The pipeline validate command doesn't require an API token
	"pipeline migrate",  // The pipeline migrate command uses a public migration API
}

// TODO: This can be deleted once we've moved over to Kong entirely, this is native and can be handles by passing context directly into the run methods
// getCommandPath returns the full path of a command
// e.g., "bk pipeline validate"
func getCommandPath(cmd *cobra.Command) string {
	var path []string
	current := cmd

	// Build path from the current command up to the root
	for current != nil {
		// Extract command name from the Use field (first word)
		name := ""
		if current.Use != "" {
			name = strings.Fields(current.Use)[0]
		}

		if name != "" {
			path = append([]string{name}, path...)
		}
		current = current.Parent()
	}

	return strings.Join(path, " ")
}

// TODO: This can be deleted once we've moved entirely over to Kong, as we've implemented the same functionality in ValidateConfiguration func
// CheckValidConfiguration returns a function that checks the viper configuration is valid to execute the command
func CheckValidConfiguration(conf *config.Config) func(cmd *cobra.Command, args []string) error {
	missingToken := conf.APIToken() == ""
	missingOrg := conf.OrganizationSlug() == ""

	var err error
	switch {
	case missingToken && missingOrg:
		err = errors.New("you must set a valid API token and organization slug. Run `bk configure`, or set the environment variables `BUILDKITE_API_TOKEN` and `BUILDKITE_ORGANIZATION_SLUG`")
	case missingToken:
		err = errors.New("you must set a valid API token. Run `bk configure`, or set the environment variable `BUILDKITE_API_TOKEN`")
	case missingOrg:
		err = errors.New("you must set a valid organization slug. Run `bk configure`, or set the environment variable `BUILDKITE_ORGANIZATION_SLUG`")
	}

	return func(cmd *cobra.Command, args []string) error {
		// Skip token check for commands that don't need it
		cmdPath := getCommandPath(cmd)

		for _, exemptCmd := range CommandsNotRequiringToken {
			// Check if the command path ends with the exempt command pattern
			if strings.HasSuffix(cmdPath, exemptCmd) {
				return nil // Skip validation for exempt commands
			}
		}

		return err
	}
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
