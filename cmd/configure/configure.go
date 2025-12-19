package configure

import (
	"context"
	"errors"

	"github.com/alecthomas/kong"
	addCmd "github.com/buildkite/cli/v3/cmd/configure/add"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type ConfigureCmd struct {
	Force bool   `help:"Force setting a new token"`
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`

	Add addCmd.AddCmd `cmd:"" help:"Add configuration for a new organization" default:"1"`
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

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	if c.Org == "" {
		return errors.New("organization slug cannot be empty")
	}

	if !c.Force && f.Config.APIToken() != "" {
		return errors.New("API token already configured. You must use --force")
	}

	return addCmd.ConfigureRun(ctx, f, c.Org, c.Token)
}
