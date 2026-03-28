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
func LoginWithToken(f *factory.Factory, org, token string) error {
	if org == "" {
		return errors.New("--org is required when --token is provided")
	}
	if token == "" {
		return errors.New("--token cannot be empty")
	}

	kr := keyring.New()
	if !kr.IsAvailable() {
		return errors.New("system keychain is not available; cannot store token")
	}
	if err := kr.Set(org, token); err != nil {
		return fmt.Errorf("failed to store token in keychain: %w", err)
	}
	fmt.Println("Token stored securely in system keychain.")

	if err := f.Config.EnsureOrganization(org); err != nil {
		return fmt.Errorf("failed to register organization in config: %w", err)
	}

	if err := f.Config.SelectOrganization(org, f.GitRepository != nil); err != nil {
		return fmt.Errorf("failed to select organization: %w", err)
	}

	return nil
}

// LoginWithSession stores an OAuth session for an organization in the system keychain.
func LoginWithSession(f *factory.Factory, org string, session *oauth.Session) error {
	if org == "" {
		return errors.New("organization cannot be empty")
	}
	if session == nil || session.AccessToken == "" {
		return errors.New("oauth session must include an access token")
	}

	kr := keyring.New()
	if !kr.IsAvailable() {
		return errors.New("system keychain is not available; cannot store token")
	}
	if err := kr.SetSession(org, session); err != nil {
		return fmt.Errorf("failed to store token in keychain: %w", err)
	}
	fmt.Println("Token stored securely in system keychain.")

	if err := f.Config.EnsureOrganization(org); err != nil {
		return fmt.Errorf("failed to register organization in config: %w", err)
	}

	if err := f.Config.SelectOrganization(org, f.GitRepository != nil); err != nil {
		return fmt.Errorf("failed to select organization: %w", err)
	}

	return nil
}

func (c *LoginCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithoutAPIClients())
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

	// Resolve scope groups (e.g. "read_only" to individual read_* scopes).
	// When --scopes is empty, NewFlow defaults to requesting the full known
	// scope set and Buildkite grants the subset the user can actually use.
	resolvedScopes := oauth.ResolveScopes(c.Scopes)

	// Create OAuth flow
	cfg := &oauth.Config{
		// Host default handled via NewFlow, omitted to allow usage of BUILDKITE_HOST
		Scopes: resolvedScopes,
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

	orgs, err := resolveOrganizationsFromToken(ctx, f.Config.RESTAPIEndpoint(), tokenResp.AccessToken)
	if err != nil {
		return err
	}

	session := tokenResp.Session(cfg.Host, cfg.ClientID, time.Now())
	if err := storeSessionForOrganizations(f, orgs, session); err != nil {
		return err
	}

	fmt.Printf("\n✅ Successfully authenticated with organization %q\n", orgs[0].Slug)
	fmt.Printf("  Scopes: %s\n", tokenResp.Scope)

	return nil
}

func resolveOrganizationsFromToken(ctx context.Context, baseURL, token string) ([]buildkite.Organization, error) {
	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(baseURL),
		buildkite.WithTokenAuth(token),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	var allOrgs []buildkite.Organization
	page := 1
	for {
		orgs, resp, err := client.Organizations.List(ctx, &buildkite.OrganizationListOptions{
			ListOptions: buildkite.ListOptions{Page: page},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list organizations: %w", err)
		}
		allOrgs = append(allOrgs, orgs...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	if len(allOrgs) == 0 {
		return nil, fmt.Errorf("no organizations found for this token")
	}

	return allOrgs, nil
}

func resolveOrganizationFromToken(ctx context.Context, baseURL, token string) (*buildkite.Organization, error) {
	orgs, err := resolveOrganizationsFromToken(ctx, baseURL, token)
	if err != nil {
		return nil, err
	}

	return &orgs[0], nil
}

func storeSessionForOrganizations(f *factory.Factory, orgs []buildkite.Organization, session *oauth.Session) error {
	if len(orgs) == 0 {
		return errors.New("no organizations found for this token")
	}
	if err := LoginWithSession(f, orgs[0].Slug, session); err != nil {
		return err
	}

	kr := keyring.New()
	seen := map[string]struct{}{orgs[0].Slug: {}}
	for _, org := range orgs[1:] {
		if org.Slug == "" {
			continue
		}
		if _, exists := seen[org.Slug]; exists {
			continue
		}
		seen[org.Slug] = struct{}{}

		if err := kr.SetSession(org.Slug, session); err != nil {
			return fmt.Errorf("failed to store token in keychain: %w", err)
		}
		if err := f.Config.EnsureOrganization(org.Slug); err != nil {
			return fmt.Errorf("failed to register organization in config: %w", err)
		}
	}

	return nil
}
