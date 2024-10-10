package user

import (
	"context"
	"fmt"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func CommandUserInvite(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite [emails]",
		Short: "Invite users to your organization",
		Long: heredoc.Doc(`
			Invite 1 or many users to your organization.
			`),
		Example: heredoc.Doc(`
			# Invite a single user to your organization
			$ bk user invite bob@supercoolorg.com

			# Invite multiple users to your organization
			$ bk user invite bob@supercoolorg.com bobs_mate@supercoolorg.com
			`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("at least one email address is required")
			}

			orgID, err := graphql.GetOrganizationID(cmd.Context(), f.GraphQLClient, f.Config.OrganizationSlug())
			if err != nil {
				return err
			}

			return createInvite(cmd.Context(), f, orgID.Organization.GetId(), args...)
		},
	}
	return cmd
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
