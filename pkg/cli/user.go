package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/factory"
)

// User commands
type UserCmd struct {
	Invite UserInviteCmd `cmd:"" help:"Invite users to your Buildkite organization via email"`
	Whoami WhoamiCmd     `cmd:"" help:"Show current user and organization information"`
}

type UserInviteCmd struct {
	Organization string   `help:"Organization slug (if omitted, uses configured organization)" placeholder:"my-org"`
	Emails       []string `arg:"" help:"Email addresses to invite"`
}

func (u *UserInviteCmd) Help() string {
	return `Examples:
  # Invite a single user
  bk user invite john@example.com
  
  # Invite multiple users
  bk user invite john@example.com jane@example.com
  
  # Invite users to a specific organization
  bk user invite --organization=acme-corp john@example.com

Users will receive an email invitation to join the organization.`
}

// User command implementations
func (u *UserInviteCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	if len(u.Emails) == 0 {
		return fmt.Errorf("at least one email address is required")
	}

	org := u.Organization
	if org == "" {
		org = f.Config.OrganizationSlug()
	}

	// Get organization ID
	orgID, err := graphql.GetOrganizationID(ctx, f.GraphQLClient, org)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Create invitations concurrently
	return createInvite(ctx, f, orgID.Organization.GetId(), u.Emails...)
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

	wg.Wait()
	close(errChan)

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


