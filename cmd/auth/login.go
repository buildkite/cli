package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	"github.com/pkg/browser"
)

type LoginCmd struct {
	Org    string `help:"Organization slug" required:""`
	Scopes string `help:"OAuth scopes to request" default:""`
}

func (c *LoginCmd) Help() string {
	return `
Authenticate with Buildkite using OAuth instead of manually creating an API token.

This command opens your browser to authenticate with Buildkite, then securely stores
the resulting token in your system keychain (macOS Keychain, Windows Credential Manager,
or Linux Secret Service).

Examples:

  # Login for a specific organization
  $ bk auth login --org my-org

  # Login with custom scopes (e.g., for cluster management)
  $ bk auth login --org my-org --scopes "read_user read_organizations read_clusters write_clusters"
`
}

func (c *LoginCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	// Check if already configured
	existingToken := f.Config.GetTokenForOrg(c.Org)
	if existingToken != "" && !globals.SkipConfirmation() {
		fmt.Printf("Organization %q is already configured. Use 'bk configure --force' to override.\n", c.Org)
		return nil
	}

	// Create OAuth flow
	cfg := &oauth.Config{
		// Host default handled via NewFlow, omitted to allow usage of BUILDKITE_HOST
		ClientID: oauth.DefaultClientID,
		OrgSlug:  c.Org,
		Scopes:   c.Scopes,
	}

	flow, err := oauth.NewFlow(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize OAuth flow: %w", err)
	}
	defer flow.Close()

	// Get authorization URL
	authURL := flow.AuthorizationURL()

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", authURL)

	// Open browser
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
	}

	// Wait for callback with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("Waiting for authentication...")

	result, err := flow.WaitForCallback(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Exchange code for token
	fmt.Println("Exchanging authorization code for token...")

	tokenResp, err := flow.ExchangeCode(ctx, result.Code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	// Store token in keyring
	kr := keyring.New()
	if kr.IsAvailable() {
		if err := kr.Set(c.Org, tokenResp.AccessToken); err != nil {
			fmt.Printf("Warning: could not store token in keychain: %v\n", err)
			fmt.Println("Falling back to config file storage.")
		} else {
			fmt.Println("Token stored securely in system keychain.")
		}
	}

	// Also store in config for fallback/compatibility
	if err := f.Config.SetTokenForOrg(c.Org, tokenResp.AccessToken); err != nil {
		return fmt.Errorf("failed to save token to config: %w", err)
	}

	// Select the organization
	if err := f.Config.SelectOrganization(c.Org, f.GitRepository != nil); err != nil {
		return fmt.Errorf("failed to select organization: %w", err)
	}

	fmt.Printf("\n✅ Successfully authenticated with organization %q\n", c.Org)
	fmt.Printf("  Scopes: %s\n", tokenResp.Scope)

	return nil
}
