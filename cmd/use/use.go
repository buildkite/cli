package use

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type UseCmd struct {
	OrganizationSlug string `arg:"" optional:"" help:"Organization slug to use"`
}

func (c *UseCmd) Help() string {
	return `Select a configured organization.

Examples:
	# Use the 'my-cool-org' configuration
	$ bk use my-cool-org

	# Interactively select an organization
	$ bk use
`
}

func (c *UseCmd) Run(globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.NoInput = globals.DisableInput()

	var org *string
	if c.OrganizationSlug != "" {
		org = &c.OrganizationSlug
	}

	return useRun(org, f.Config, f.GitRepository != nil, f.NoInput)
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
