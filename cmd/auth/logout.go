package auth

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

type LogoutCmd struct {
	All bool   `help:"Log out of all organizations" xor:"target"`
	Org string `help:"Organization slug (defaults to currently selected organization)" optional:"" xor:"target"`
}

func (c *LogoutCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	if c.All {
		return c.logoutAll(f)
	}

	return c.logoutOrg(f)
}

func (c *LogoutCmd) logoutAll(f *factory.Factory) error {
	orgs := f.Config.ConfiguredOrganizations()

	kr := keyring.New()
	if kr.IsAvailable() {
		for _, org := range orgs {
			if err := kr.Delete(org); err != nil {
				fmt.Printf("Warning: could not remove token from keychain for %q: %v\n", org, err)
			}
		}
	}

	if err := f.Config.ClearAllOrganizations(); err != nil {
		return fmt.Errorf("failed to clear organizations from config: %w", err)
	}

	fmt.Printf("Logged out of all %d organizations\n", len(orgs))
	return nil
}

func (c *LogoutCmd) logoutOrg(f *factory.Factory) error {
	org := c.Org
	if org == "" {
		org = f.Config.OrganizationSlug()
	}

	if org == "" {
		return fmt.Errorf("no organization specified and none currently selected")
	}

	kr := keyring.New()
	if kr.IsAvailable() {
		if err := kr.Delete(org); err != nil {
			fmt.Printf("Warning: could not remove token from keychain: %v\n", err)
		} else {
			fmt.Println("Token removed from system keychain.")
		}
	}

	if err := f.Config.SetTokenForOrg(org, ""); err != nil {
		return fmt.Errorf("failed to clear token from config: %w", err)
	}

	fmt.Printf("Logged out of organization %q\n", org)
	return nil
}
