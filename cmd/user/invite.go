package user

import (
	"context"
	"fmt"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type InviteCmd struct {
	Emails []string `arg:"" required:"" help:"Email addresses to invite"`
}

func (c *InviteCmd) Help() string {
	return `
Examples:
  # Invite a single user to your organization
  $ bk user invite bob@supercoolorg.com

  # Invite multiple users to your organization
  $ bk user invite bob@supercoolorg.com bobs_mate@supercoolorg.com
`
}

func (c *InviteCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	orgID, err := graphql.GetOrganizationID(ctx, f.GraphQLClient, f.Config.OrganizationSlug())
	if err != nil {
		return err
	}

	return createInvite(ctx, f, orgID.Organization.GetId(), c.Emails...)
}

func createInvite(ctx context.Context, f *factory.Factory, orgID string, emails ...string) error {
	if len(emails) == 0 {
		return nil
	}

	errChan := make(chan error, len(emails))
	var wg sync.WaitGroup

	for _, email := range emails {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			_, err := graphql.InviteUser(ctx, f.GraphQLClient, orgID, []string{email})
			if err != nil {
				errChan <- fmt.Errorf("error creating user invite for %s: %w", email, err)
			}
		}(email)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors creating user invites: %v", errs)
	}

	message := "Invite sent to"
	if len(emails) > 1 {
		message = "Invites sent to"
	}

	fmt.Printf("%s: %v\n", message, emails)
	return nil
}
