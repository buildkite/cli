package configure

import (
	"context"
	"errors"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type ConfigureCmd struct {
	Force bool   `help:"Force setting a new token"`
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`

	Add AddCmd `cmd:"" help:"Add config for new organization" subcmd:"add"`
}

func (c *ConfigureCmd) Help() string {
	return `
Examples:
  # Configure Buildkite API token
  $ bk configure --org my-org --token my-token

  # Force setting a new token
  $ bk configure --force --org my-org --token my-token
`
}

func (c *ConfigureCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()

	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if !c.Force && f.Config.APIToken() != "" {
		return errors.New("API token already configured. You must use --force")
	}

	ctx := context.Background()
	if c.Org != "" && c.Token != "" {
		return ConfigureWithCredentials(ctx, f, c.Org, c.Token)

	}
	return ConfigureRun(ctx, f)
}
