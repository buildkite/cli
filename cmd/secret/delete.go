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
)

type DeleteCmd struct {
	ClusterUUID string `help:"The UUID of the cluster" required:"" name:"cluster-uuid"`
	SecretUUID  string `help:"The UUID of the secret to delete" required:"" name:"secret-uuid" aliases:"secret-id"`
}

func (c *DeleteCmd) Help() string {
	return `
Delete a secret from a cluster.

You will be prompted to confirm deletion unless --yes is set.

Examples:
  # Delete a secret (with confirmation prompt)
  $ bk secret delete --cluster-uuid my-cluster-uuid --secret-uuid my-secret-uuid

  # Delete a secret without confirmation
  $ bk secret delete --cluster-uuid my-cluster-uuid --secret-uuid my-secret-uuid --yes
`
}

func (c *DeleteCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Are you sure you want to delete secret %s?", c.SecretUUID))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Deletion cancelled.")
		return nil
	}

	if err = bkIO.SpinWhile(f, "Deleting secret", func() error {
		_, err = f.RestAPIClient.ClusterSecrets.Delete(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.SecretUUID)
		return err
	}); err != nil {
		return fmt.Errorf("error deleting secret: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Secret deleted successfully.")
	return nil
}
