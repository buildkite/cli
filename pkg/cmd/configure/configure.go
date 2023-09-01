package configure

import (
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdConfigure(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "configure",
		Aliases: []string{"config"},
		Args:    cobra.NoArgs,
		Short:   "Configure Buildkite API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && f.Config.APIToken != "" {
				return errors.New("API token already configured. You must use --force.")
			}

			qs := []*survey.Question{
				{
					Name:     "org",
					Prompt:   &survey.Input{Message: "What is your organization slug?"},
					Validate: survey.Required,
				},
				{
					Name:     "token",
					Prompt:   &survey.Password{Message: "Paste your API token:"},
					Validate: survey.Required,
				},
			}
			answers := struct{ Org, Token string }{}

			err := survey.Ask(qs, &answers)
			if err != nil {
				return err
			}

			f.Config.APIToken = answers.Token
			f.Config.Organization = answers.Org

			return f.Config.Save()
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")

	return cmd
}
