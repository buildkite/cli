package init

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdInit(viper *viper.Viper) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Args:  cobra.ExactArgs(0),
		Short: "Initialise authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && viper.IsSet(config.APIToken) {
				return errors.New("API token already configured. You mused use --force.")
			}

			var token string
			prompt := &survey.Password{
				Message: "Paste your API token:",
			}

			err := survey.AskOne(prompt, &token)
			if err != nil {
				return err
			}

			viper.Set(config.APIToken, token)
			return viper.WriteConfig()
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")

	return cmd
}
