package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
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

By default, the CLI requests all available scopes and Buildkite grants only those
your account has permission for. Use --scopes to request a specific subset instead.

Scope groups can be used as shorthand for common permission sets:
  read_only    All read_* scopes (read-only access)

Groups can be mixed with individual scopes:
  --scopes "read_only write_builds"

Examples:

  # Login with full permissions (inherits your account's scopes)
  $ bk auth login

  # Login non-interactively with an API token
  $ bk auth login --org my-org --token my-token

  # Login with read-only access
  $ bk auth login --scopes read_only

  # Login with read-only plus write access to builds
  $ bk auth login --scopes "read_only write_builds"

  # Login with specific scopes
  $ bk auth login --scopes "read_user read_organizations read_clusters write_clusters"
`
}

// LoginWithToken stores a token for an organization in the system keychain.
// When the keychain is unavailable (e.g. BUILDKITE_NO_KEYRING=1 is set), it
// still registers the org and selects it in config so that commands resolve the
// org correctly; the caller is expected to supply the token via BUILDKITE_API_TOKEN.
func LoginWithToken(f *factory.Factory, org, token string) error {
	if org == "" {
		return errors.New("--org is required when --token is provided")
	}
	if token == "" {
		return errors.New("--token cannot be empty")
	}

	kr := keyring.New()
	if kr.IsAvailable() {
		if err := kr.Set(org, token); err != nil {
			return fmt.Errorf("failed to store token in keychain: %w", err)
		}
		fmt.Println("Token stored securely in system keychain.")
	} else {
		fmt.Println("Keychain unavailable; token not stored. Use BUILDKITE_API_TOKEN to supply your token at runtime.")
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

	// Resolve scope groups (e.g., "read_only" → individual read_* scopes).
	// When --scopes is empty, no scope parameter is sent and the token
	// inherits the user's full Buildkite permissions.
	resolvedScopes := oauth.ResolveScopes(c.Scopes)

	// Create OAuth flow
	cfg := &oauth.Config{
		// Host default handled via NewFlow, omitted to allow usage of BUILDKITE_HOST
		ClientID: oauth.DefaultClientID,
		Scopes:   resolvedScopes,
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
	client, err := buildkite.NewOpts(
		buildkite.WithTokenAuth(tokenResp.AccessToken),
		buildkite.WithBaseURL(f.Config.RESTAPIEndpoint()),
	)
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

	// Store refresh token if the server issued one
	if tokenResp.RefreshToken != "" {
		kr := keyring.New()
		if kr.IsAvailable() {
			if err := kr.SetRefreshToken(org.Slug, tokenResp.RefreshToken); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store refresh token: %v\n", err)
			}
		}
	}

	fmt.Printf("\n✅ Successfully authenticated with organization %q\n", org.Slug)
	fmt.Printf("  Scopes: %s\n", tokenResp.Scope)
	if tokenResp.RefreshToken != "" {
		fmt.Printf("  Token expires in: %s (will refresh automatically)\n", formatDuration(tokenResp.ExpiresIn))
	}

	return nil
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "unknown"
	}
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}
