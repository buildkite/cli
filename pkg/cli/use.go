package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/internal/config"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/factory"
	"github.com/mattn/go-isatty"
)

// Use command
type UseCmd struct {
	Organization string `arg:"" optional:"" help:"Organization to switch to" placeholder:"my-org"`
}

func (u *UseCmd) Help() string {
	return `Examples:
  # Switch to a specific organization
  bk use my-company
  
  # Switch to another organization
  bk use acme-corp
  
  # Run without argument to choose from configured organizations
  bk use

The organization slug is saved to your configuration file and will be used 
for subsequent commands until you switch to a different organization.`
}

func (u *UseCmd) Run(ctx context.Context, f *factory.Factory) error {
	return useRun(u.Organization, f.Config, f.GitRepository != nil)
}

func useRun(orgArg string, conf *config.Config, inGitRepo bool) error {
	var selected string

	// if no organization provided
	if orgArg == "" {
		// if TTY, prompt to choose from configured orgs
		if isatty.IsTerminal(os.Stdout.Fd()) {
			var err error
			selected, err = bk_io.PromptForOne("organization", conf.ConfiguredOrganizations())
			if err != nil {
				return err
			}
		} else {
			// if not TTY, list configured organizations
			orgs := conf.ConfiguredOrganizations()
			if len(orgs) == 0 {
				return fmt.Errorf("no organizations configured. run `bk configure` to add one")
			}
			for _, org := range orgs {
				fmt.Println(org)
			}
			return nil
		}
	} else {
		selected = orgArg
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
