package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
	buildkite "github.com/buildkite/go-buildkite/v4"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	"github.com/pkg/browser"
)

type LoginCmd struct {
	Scopes string `help:"OAuth scopes to request" default:""`
	Org    string `help:"Organization slug (required with --token)" optional:""`
	Token  string `help:"API token to store (non-OAuth login)" optional:""`
}

func (c *LoginCmd) Help() string {
	return `
Authenticate with Buildkite using OAuth instead of manually creating an API token.

This command opens your browser to authenticate with Buildkite. After you select an
organization in the browser, the CLI automatically detects which organization was
authorized and stores the token securely in your system keychain (macOS Keychain,
Windows Credential Manager, or Linux Secret Service).

Examples:

  # Login (select organization in browser)
  $ bk auth login

  # Login non-interactively with an API token
  $ bk auth login --org my-org --token my-token

  # Login with custom scopes (e.g., for cluster management)
  $ bk auth login --scopes "read_user read_organizations read_clusters write_clusters"
`
}

// LoginWithToken stores a token for an organization.
// Keychain is preferred. Config file is used when keychain is unavailable
// or not writable.
func LoginWithToken(f *factory.Factory, org, token string) error {
	if org == "" {
		return errors.New("--org is required when --token is provided")
	}
	if token == "" {
		return errors.New("--token cannot be empty")
	}

	kr := keyring.New()
	wroteToKeychain := false
	if kr.IsAvailable() {
		if err := kr.Set(org, token); err != nil {
			fmt.Printf("Warning: could not store token in keychain: %v\n", err)
			fmt.Println("Falling back to config file storage.")
		} else {
			wroteToKeychain = true
			fmt.Println("Token stored securely in system keychain.")
		}
	}

	if !wroteToKeychain {
		if err := f.Config.SetTokenForOrg(org, token); err != nil {
			return fmt.Errorf("failed to save token to config: %w", err)
		}
	}

	if err := f.Config.EnsureOrganization(org); err != nil {
		return fmt.Errorf("failed to register organization in config: %w", err)
	}

	if err := f.Config.SelectOrganization(org, f.GitRepository != nil); err != nil {
		return fmt.Errorf("failed to select organization: %w", err)
	}

	return nil
}

func (c *LoginCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	if c.Token != "" {
		if err := LoginWithToken(f, c.Org, c.Token); err != nil {
			return err
		}

		fmt.Printf("\nSuccessfully authenticated with organization %q\n", c.Org)
		return nil
	}

	if c.Org != "" {
		return errors.New("--org requires --token. Use `bk auth login` for OAuth or `bk auth login --org <org> --token <token>` for token login")
	}

	// Create OAuth flow
	cfg := &oauth.Config{
		// Host default handled via NewFlow, omitted to allow usage of BUILDKITE_HOST
		ClientID: oauth.DefaultClientID,
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

	// Resolve org from the API using the new token
	client, err := buildkite.NewOpts(buildkite.WithTokenAuth(tokenResp.AccessToken))
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	orgs, _, err := client.Organizations.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}
	if len(orgs) == 0 {
		return fmt.Errorf("no organizations found for this token")
	}

	org := orgs[0]

	if err := LoginWithToken(f, org.Slug, tokenResp.AccessToken); err != nil {
		return err
	}

	fmt.Printf("\n✅ Successfully authenticated with organization %q\n", org.Slug)
	fmt.Printf("  Scopes: %s\n", tokenResp.Scope)

	return nil
}
