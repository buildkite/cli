package use

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdUse(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "use <organization>",
		Args:                  cobra.RangeArgs(0, 1),
		DisableFlagsInUseLine: true,
		Short:                 "Select an organization",
		Long:                  "Select a configured organization.",
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

func useRun(selected *string, conf *config.Config) error {
	// prompt to choose from configured orgs
	if selected == nil {
		m := conf.V.GetStringMap(config.OrganizationsSlugConfigKey)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		q := &survey.Select{
			Options: keys,
		}
		selected = new(string)
		err := survey.AskOne(q, selected)
		if err != nil {
			return err
		}
	}

	// if already selected, do nothing
	if conf.Organization == *selected {
		fmt.Printf("Using configuration for `%s`\n", *selected)
		return nil
	}

	// if the selected org exists, use it
	m := conf.V.GetStringMap(config.OrganizationsSlugConfigKey)
	if org, ok := m[*selected]; ok {
		conf.Organization = *selected
		conf.APIToken = org.(map[string]interface{})[config.APITokenConfigKey].(string)
		fmt.Printf("Using configuration for `%s`\n", *selected)
		return conf.Save()
	}

	// if the selected org doesnt exist, recommend configuring it and error out
	return fmt.Errorf("No configuration found for `%s`. Run `bk configure` to add it.", *selected)
}
