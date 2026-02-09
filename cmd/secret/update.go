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

type UpdateCmd struct {
	ClusterID   string `help:"The ID of the cluster" required:"" name:"cluster-id"`
	SecretID    string `help:"The UUID of the secret to update" required:"" name:"secret-id"`
	Description string `help:"Update the description of the secret" optional:""`
	Policy      string `help:"Update the access policy for the secret (YAML format)" optional:""`
	UpdateValue bool   `help:"Prompt to update the secret value" optional:"" name:"update-value"`
	Output      string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *UpdateCmd) Help() string {
	return `
Update a cluster secret's description, policy, or value.

Use --update-value to be prompted for a new secret value (input will be masked).

Examples:
  # Update a secret's description
  $ bk secret update --cluster-id my-cluster-id --secret-id my-secret-id --description "New description"

  # Update a secret's value
  $ bk secret update --cluster-id my-cluster-id --secret-id my-secret-id --update-value

  # Update both description and value
  $ bk secret update --cluster-id my-cluster-id --secret-id my-secret-id --description "New description" --update-value
`
}

func (c *UpdateCmd) Validate() error {
	if c.Description == "" && c.Policy == "" && !c.UpdateValue {
		return fmt.Errorf("at least one of --description, --policy, or --update-value must be provided")
	}
	return nil
}

func (c *UpdateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	org := f.Config.OrganizationSlug()

	if c.UpdateValue {
		if f.NoInput {
			return fmt.Errorf("--update-value requires interactive input but --no-input is set")
		}
		fmt.Fprint(os.Stderr, "Enter new secret value: ")
		value, err := bkIO.ReadPassword()
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("error reading secret value: %v", err)
		}
		if value == "" {
			return fmt.Errorf("secret value cannot be empty")
		}

		spinErr := bkIO.SpinWhile(f, "Updating secret value", func() {
			_, err = f.RestAPIClient.ClusterSecrets.UpdateValue(ctx, org, c.ClusterID, c.SecretID, buildkite.ClusterSecretValueUpdate{
				Value: value,
			})
		})
		if spinErr != nil {
			return spinErr
		}
		if err != nil {
			return fmt.Errorf("error updating secret value: %v", err)
		}
	}

	var secret buildkite.ClusterSecret
	if c.Description != "" || c.Policy != "" {
		spinErr := bkIO.SpinWhile(f, "Updating secret", func() {
			secret, _, err = f.RestAPIClient.ClusterSecrets.Update(ctx, org, c.ClusterID, c.SecretID, buildkite.ClusterSecretUpdate{
				Description: c.Description,
				Policy:      c.Policy,
			})
		})
		if spinErr != nil {
			return spinErr
		}
		if err != nil {
			return fmt.Errorf("error updating secret: %v", err)
		}
	} else {
		// Fetch the secret to display current state
		spinErr := bkIO.SpinWhile(f, "Loading secret", func() {
			secret, _, err = f.RestAPIClient.ClusterSecrets.Get(ctx, org, c.ClusterID, c.SecretID)
		})
		if spinErr != nil {
			return spinErr
		}
		if err != nil {
			return fmt.Errorf("error fetching secret: %v", err)
		}
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	secretView := output.Viewable[buildkite.ClusterSecret]{
		Data:   secret,
		Render: renderSecretText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, secretView, format)
	}

	fmt.Fprintln(os.Stderr, "Secret updated successfully.")
	fmt.Fprintln(os.Stdout)
	return output.Write(os.Stdout, secretView, format)
}
