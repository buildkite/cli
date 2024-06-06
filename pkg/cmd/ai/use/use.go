package use

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdUse(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "use [token key]",
		Args:                  cobra.RangeArgs(0, 1),
		DisableFlagsInUseLine: true,
		Short:                 "Select an OpenAI token to use.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var org *string
			if len(args) > 0 {
				org = &args[0]
			}
			return useRun(org, f.Config)
		},
	}

	return cmd
}

func useRun(tokenKey *string, conf *config.Config) error {
	var selected string

	// prompt to choose from configured orgs if one is not already selected
	if tokenKey == nil {
		var err error
		selected, err = io.PromptForOne(conf.ConfiguredOpenAITokens())
		if err != nil {
			return err
		}
	} else {
		selected = *tokenKey
	}

	// if already selected, do nothing
	if conf.OpenAIToken() == selected {
		fmt.Printf("Using OpenAI token named `%s`\n", selected)
		return nil
	}

	// if the selected org exists, use it
	if conf.HasConfiguredOpenAITokens(selected) {
		fmt.Printf("Using OpenAI token named `%s`\n", selected)
		return conf.SelectOpenAIToken(selected)
	}

	// if the selected org doesnt exist, recommend configuring it and error out
	return fmt.Errorf("No token named %s found. Run `bk ai configure` to add it.", selected)
}
