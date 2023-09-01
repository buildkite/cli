package validation

import (
	"errors"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/cobra"
)

// CheckValidConfiguration returns a function that checks the viper configuration is valid to execute the command
func CheckValidConfiguration(conf *config.Config) func(cmd *cobra.Command, args []string) error {
	var err error

	// ensure the configuration has an API token set
	if conf.APIToken == "" || conf.Organization == "" {
		err = errors.New("You must set a valid API token. Run `bk configure`.")
	}

	return func(cmd *cobra.Command, args []string) error {
		return err
	}
}
