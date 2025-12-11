package whoami

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/buildkite/go-buildkite/v4"
)

type WhoAmIOutput struct {
	OrganizationSlug string                `json:"organization_slug"`
	Token            buildkite.AccessToken `json:"token"`
}

func (w WhoAmIOutput) TextOutput() string {
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

type WhoAmICmd struct {
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *WhoAmICmd) Help() string {
	return `
It returns information on the current session.

Examples:
	# List the current token session
	$ bk whoami

	# List the current token session in JSON format
	$ bk whoami -o json
`
}

func (c *WhoAmICmd) Run(kongCtx *kong.Context) error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	format := output.Format(c.Output)
	if format != output.FormatJSON && format != output.FormatYAML && format != output.FormatText {
		return fmt.Errorf("invalid output format: %s", c.Output)
	}

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

	w := WhoAmIOutput{
		OrganizationSlug: orgSlug,
		Token:            token,
	}

	err = output.Write(os.Stdout, w, format)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
