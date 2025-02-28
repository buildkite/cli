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
}

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

// CheckValidConfiguration returns a function that checks the viper configuration is valid to execute the command
func CheckValidConfiguration(conf *config.Config) func(cmd *cobra.Command, args []string) error {
	var err error

	// ensure the configuration has an API token set
	if conf.APIToken() == "" || conf.OrganizationSlug() == "" {
		err = errors.New("You must set a valid API token. Run `bk configure`.")
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
