package add

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAIAdd(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Args:  cobra.NoArgs,
		Short: "Add an OpenAI token.",
		Long:  "Add an OpenAI token for use with CLI AI tooling",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ConfigureRun(f)
		},
	}

	return cmd
}

func ConfigureRun(f *factory.Factory) error {
	qs := []*survey.Question{
		{
			Name:     "key",
			Prompt:   &survey.Input{Message: "Provide a name for the token:"},
			Validate: survey.Required,
		},
		{
			Name:     "token",
			Prompt:   &survey.Password{Message: "Paste your OpenAI token:"},
			Validate: survey.Required,
		},
	}
	answers := struct{ Key, Token string }{}

	err := survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	err = f.Config.SetOpenAIToken(answers.Key, answers.Token)

	return err
}
