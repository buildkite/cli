package add

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAdd(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Args:  cobra.NoArgs,
		Short: "Add config for new organization",
		Long:  "Add configuration for a new organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ConfigureRun(f)
		},
	}

	return cmd
}

func ConfigureRun(f *factory.Factory) error {
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
}
