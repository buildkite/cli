package configure

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdConfigure(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "configure",
		Args:  cobra.NoArgs,
		Short: "Configure Buildkite API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && f.Config.IsSet(config.APITokenConfigKey) {
				return errors.New("API token already configured. You must use --force.")
			}

			var token string
			prompt := &survey.Password{
				Message: "Paste your API token:",
			}

			err := survey.AskOne(prompt, &token)
			if err != nil {
				return err
			}

			f.Config.Set(config.APITokenConfigKey, token)
			return f.Config.WriteConfig()
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")
	return cmd
}
