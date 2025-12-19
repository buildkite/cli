package configure

import (
	"errors"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/cmd/configure/add"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type ConfigureCmd struct {
	Show ConfigureShowCmd `cmd:"" help:"Configure Buildkite API token" default:"1" hidden:"" kong:"-"`
	Add  add.AddCmd       `cmd:"" optional:"" help:"Add configuration for a new organization"`
}

type ConfigureShowCmd struct {
	Default bool   `cmd:"" help:"Configure Buildkite API token with provided arguments" default:"true" hidden:"" kong:"-"`
	Org     string `help:"Organization slug"`
	Token   string `help:"API token"`
	Force   bool   `help:"Force setting a new token"`
}

func (c *ConfigureShowCmd) Help() string {
	var help strings.Builder
	help.WriteString(`
Examples:
  # Configure Buildkite API token
  $ bk configure --org my-org --token my-token

  # Force setting a new token
  $ bk configure --force --org my-org --token my-token 
`)

	return help.String()
}

func (c *ConfigureShowCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	if !c.Force && f.Config.APIToken() != "" {
		return errors.New("API token already configured. You must use --force")
	}

	// If flags are provided, use them directly
	if c.Org != "" && c.Token != "" {
		return add.ConfigureWithCredentials(f, c.Org, c.Token)
	}

	return add.ConfigureRun(f, c.Org)
}
