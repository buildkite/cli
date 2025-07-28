package cli

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/pkg/factory"
)

// Whoami command
type WhoamiCmd struct {
	OutputFlag
}

func (w *WhoamiCmd) Help() string {
	return `Shows information about the current user and organization based on the configured API token.

EXAMPLES:
  # Show current user and organization
  bk whoami

  # Show information in JSON format
  bk whoami --output json`
}

type whoamiOutput struct {
	OrganizationSlug string `json:"organization_slug"`
	Token            struct {
		UUID        string   `json:"uuid"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
		User        struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	} `json:"token"`
}

func (w *WhoamiCmd) Run(ctx context.Context, f *factory.Factory) error {
	w.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	orgSlug := f.Config.OrganizationSlug()
	if orgSlug == "" {
		orgSlug = "<None>"
	}

	token, _, err := f.RestAPIClient.AccessTokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	output := whoamiOutput{
		OrganizationSlug: orgSlug,
	}
	output.Token.UUID = token.UUID
	output.Token.Description = token.Description
	output.Token.Scopes = token.Scopes
	output.Token.User.Name = token.User.Name
	output.Token.User.Email = token.User.Email

	if ShouldUseStructuredOutput(f) {
		return Print(output, f)
	}

	fmt.Printf("Current organization: %s\n", output.OrganizationSlug)
	fmt.Println()
	fmt.Printf("API Token UUID:        %s\n", output.Token.UUID)
	fmt.Printf("API Token Description: %s\n", output.Token.Description)
	fmt.Printf("API Token Scopes:      %v\n", output.Token.Scopes)
	fmt.Println()
	fmt.Printf("API Token user name:  %s\n", output.Token.User.Name)
	fmt.Printf("API Token user email: %s\n", output.Token.User.Email)
	return nil
}
