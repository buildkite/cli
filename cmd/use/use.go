package use

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type UseCmd struct {
	Organization string `arg:"" optional:"" help:"Organization slug to use"`
}

func (c *UseCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	var org *string
	if c.Organization != "" {
		org = &c.Organization
	}

	return useRun(org, f.Config, f.GitRepository != nil, globals.DisableInput())
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
