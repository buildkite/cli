package auth

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type SwitchCmd struct {
	OrganizationSlug string `arg:"" optional:"" help:"Organization slug to switch"`
}

func (c *SwitchCmd) Help() string {
	return `Select a configured organization.

Examples:
	# Switch the 'my-cool-org' configuration
	$ bk auth switch my-cool-org

	# Interactively select an organization
	$ bk auth switch
`
}

func (c *SwitchCmd) Run(globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.NoInput = globals.DisableInput()

	var org *string
	if c.OrganizationSlug != "" {
		org = &c.OrganizationSlug
	}

	return switchRun(org, f.Config, f.GitRepository != nil, f.NoInput)
}

func switchRun(org *string, conf *config.Config, inGitRepo bool, noInput bool) error {
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

	// if the selected org exists, switch it
	if conf.HasConfiguredOrganization(selected) {
		fmt.Printf("Using configuration for `%s`\n", selected)
		return conf.SelectOrganization(selected, inGitRepo)
	}

	// if the selected org doesnt exist, recommend configuring it and error out
	return fmt.Errorf("no configuration found for `%s`. run `bk auth login` to add it", selected)
}
