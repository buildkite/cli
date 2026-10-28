package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type TokenCmd struct {
	JSON    bool          `help:"Inspect the current token and print its value and expiry as JSON."`
	Timeout time.Duration `help:"Timeout for token inspection and renewal requests." default:"30s"`
}

type tokenInspection struct {
	AccessToken      string     `json:"access_token"`
	ExpiresAt        *time.Time `json:"expires_at"`
	OrganizationSlug string     `json:"organization_slug"`
}

func (c *TokenCmd) Help() string {
	return `
Prints the stored API token for the currently selected organization to stdout.

The token is retrieved from the configured credential store (or the
BUILDKITE_API_TOKEN environment variable if set). This is useful for passing
the token to other tools.

With --json, the CLI inspects the token through its normal API transport. If
the inspection returns 401, the transport can renew stored OAuth credentials
and retry the inspection. The output contains the token used for the final
response, that response's expiry when present, and the selected organization.

Inspection does not guarantee a minimum remaining lifetime. A token can expire
or be revoked before another tool uses it. A null expiry means the API did not
report one; it does not identify the credential as static or renewable.
--timeout bounds network requests, but credential-store operations
can outlast the deadline.

Examples:
	# Print the current token
	$ bk auth token

	# Inspect the current token and its expiry
	$ bk auth token --json

	# Use the token in a curl request
	$ curl -H "Authorization: Bearer $(bk auth token)" https://api.buildkite.com/v2/user
`
}

func (c *TokenCmd) Validate() error {
	if c.JSON && c.Timeout <= 0 {
		return fmt.Errorf("--timeout must be positive")
	}
	return nil
}

func (c *TokenCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	token := f.Config.APIToken()
	if token == "" {
		return fmt.Errorf("no token found; run `bk auth login` to authenticate")
	}

	if c.JSON {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, c.Timeout)
		defer cancel()

		inspection, err := inspectToken(ctx, f)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(inspection)
	}

	fmt.Fprintln(os.Stdout, token)
	return nil
}

func inspectToken(ctx context.Context, f *factory.Factory) (tokenInspection, error) {
	details, response, err := f.RestAPIClient.AccessTokens.Get(ctx)
	if err != nil {
		return tokenInspection{}, fmt.Errorf("failed to inspect access token: %w", err)
	}
	if response == nil || response.Request == nil {
		return tokenInspection{}, fmt.Errorf("token inspection did not identify its request")
	}
	scheme, token, ok := strings.Cut(response.Request.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return tokenInspection{}, fmt.Errorf("token inspection request did not use bearer authentication")
	}

	inspection := tokenInspection{
		AccessToken:      token,
		OrganizationSlug: f.Config.OrganizationSlug(),
	}
	if details.ExpiresAt != nil {
		inspection.ExpiresAt = &details.ExpiresAt.Time
	}
	return inspection, nil
}
