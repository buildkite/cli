package secret

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CreateCmd struct {
	ClusterID   string `help:"The ID of the cluster" required:"" name:"cluster-id"`
	Key         string `help:"The key name for the secret (e.g. MY_SECRET)" required:""`
	Value       string `help:"The secret value. If not provided, you will be prompted to enter it." optional:""`
	Description string `help:"A description of the secret" optional:""`
	Policy      string `help:"The access policy for the secret (YAML format)" optional:""`
	Output      string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *CreateCmd) Help() string {
	return `
Create a new secret in a cluster.

If --value is not provided, you will be prompted to enter the secret value
interactively (input will be masked).

Examples:
  # Create a secret with interactive value input
  $ bk secret create --cluster-id my-cluster-id --key MY_SECRET

  # Create a secret with the value provided inline
  $ bk secret create --cluster-id my-cluster-id --key MY_SECRET --value "s3cr3t"

  # Create a secret with a description
  $ bk secret create --cluster-id my-cluster-id --key MY_SECRET --description "My secret description"
`
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	value := c.Value
	if value == "" {
		if f.NoInput {
			return fmt.Errorf("--value is required when --no-input is set")
		}
		fmt.Fprint(os.Stderr, "Enter secret value: ")
		value, err = bkIO.ReadPassword()
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("error reading secret value: %v", err)
		}
		if value == "" {
			return fmt.Errorf("secret value cannot be empty")
		}
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	input := buildkite.ClusterSecretCreate{
		Key:         c.Key,
		Value:       value,
		Description: c.Description,
		Policy:      c.Policy,
	}

	var secret buildkite.ClusterSecret
	spinErr := bkIO.SpinWhile(f, "Creating secret", func() {
		secret, _, err = f.RestAPIClient.ClusterSecrets.Create(ctx, f.Config.OrganizationSlug(), c.ClusterID, input)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error creating secret: %v", err)
	}

	secretView := output.Viewable[buildkite.ClusterSecret]{
		Data:   secret,
		Render: renderSecretText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, secretView, format)
	}

	fmt.Fprintf(os.Stdout, "Secret %s created successfully\n\n", secret.Key)
	return output.Write(os.Stdout, secretView, format)
}
