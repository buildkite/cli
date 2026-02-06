package auth

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

type LogoutCmd struct {
	Org string `help:"Organization slug (defaults to currently selected organization)" optional:""`
}

func (c *LogoutCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

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
