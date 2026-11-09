package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

type LogoutCmd struct {
	All bool   `help:"Log out of all organizations"`
	Org string `kong:"-"` // Supplied by the global --org flag.
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
			if err := deleteCredentials(kr, org); err != nil {
				return fmt.Errorf("could not remove credentials for %q: %w", org, err)
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
		if err := deleteCredentials(kr, org); err != nil {
			return fmt.Errorf("could not remove credentials: %w", err)
		}
		fmt.Println("Token removed from credential store.")
	}

	fmt.Printf("Logged out of organization %q\n", org)
	return nil
}

type logoutTokenStore interface {
	Lock(context.Context, string) (func(), error)
	Delete(string) error
	DeleteRefreshToken(string) error
}

func deleteCredentials(kr logoutTokenStore, org string) error {
	release, err := kr.Lock(context.Background(), org)
	if err != nil {
		return err
	}
	defer release()
	// Attempt both deletions even if one backend fails.
	accessErr := kr.Delete(org)
	refreshErr := kr.DeleteRefreshToken(org)
	return errors.Join(accessErr, refreshErr)
}
