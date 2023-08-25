package configure

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdConfigure(v *viper.Viper) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "configure",
		Args:  cobra.ExactArgs(0),
		Short: "Configure Buildkite API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && v.IsSet(config.APIToken) {
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

			v.Set(config.APIToken, token)
			return v.WriteConfig()
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")
	return cmd
}
