// Package auth handles commands related to authentication via the CLI
package auth

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/buildkite/go-buildkite/v4"
)

type StatusOutput struct {
	OrganizationSlug string                `json:"organization_slug"`
	Token            buildkite.AccessToken `json:"token"`
}

func (w StatusOutput) TextOutput() string {
	b := strings.Builder{}

	b.WriteString(fmt.Sprintf("Current organization: %s\n", w.OrganizationSlug))
	b.WriteRune('\n')
	b.WriteString(fmt.Sprintf("API Token UUID:        %s\n", w.Token.UUID))
	b.WriteString(fmt.Sprintf("API Token Description: %s\n", w.Token.Description))
	b.WriteString(fmt.Sprintf("API Token Scopes:      %v\n", w.Token.Scopes))
	b.WriteRune('\n')
	b.WriteString(fmt.Sprintf("API Token user name:  %s\n", w.Token.User.Name))
	b.WriteString(fmt.Sprintf("API Token user email: %s\n", w.Token.User.Email))

	return b.String()
}

type StatusCmd struct {
	output.OutputFlags
}

func (c *StatusCmd) Help() string {
	return `
It returns information on the current session.

Examples:
	# List the current token session
	$ bk auth status
`
}

func (c *StatusCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	if validationErr := validation.ValidateConfiguration(f.Config, kongCtx.Command()); validationErr != nil {
		return err
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	orgSlug := f.Config.OrganizationSlug()

	if orgSlug == "" {
		orgSlug = "<None>"
	}

	token, _, err := f.RestAPIClient.AccessTokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	w := StatusOutput{
		OrganizationSlug: orgSlug,
		Token:            token,
	}

	err = output.Write(os.Stdout, w, format)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
