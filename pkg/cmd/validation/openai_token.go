package validation

import (
	"errors"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/cobra"
)

func OpenAITokenConfigured(conf *config.Config) func(cmd *cobra.Command, args []string) error {
	var err error
	if conf.GetOpenAIToken() == "" {
		err = errors.New("You must set an OpenAI API token to use this command. Run `bk ai configure`.")
	}
	return func(cmd *cobra.Command, args []string) error {
		return err
	}
}
