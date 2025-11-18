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
			return useRun(org, f.Config, f.GitRepository != nil, f.NoInput)
		},
	}

	return cmd
}

func useRun(org *string, conf *config.Config, inGitRepo bool, noInput bool) error {
	var selected string

	// prompt to choose from configured orgs if one is not already selected
	if org == nil {
		var err error
		selected, err = io.PromptForOne("organization", conf.ConfiguredOrganizations(), noInput)
		if err != nil {
			return err
		}
	} else {
		selected = *org
	}

	// if already selected, do nothing
	if conf.OrganizationSlug() == selected {
		fmt.Printf("Using configuration for `%s`\n", selected)
		return nil
	}

	// if the selected org exists, use it
	if conf.HasConfiguredOrganization(selected) {
		fmt.Printf("Using configuration for `%s`\n", selected)
		return conf.SelectOrganization(selected, inGitRepo)
	}

	// if the selected org doesnt exist, recommend configuring it and error out
	return fmt.Errorf("no configuration found for `%s`. run `bk configure` to add it", selected)
}
