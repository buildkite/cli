package maintainer

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
	ClusterUUID  string `arg:"" help:"Cluster UUID" name:"cluster-uuid"`
	MaintainerID string `arg:"" help:"Maintainer ID to delete" name:"maintainer-id"`
}

func (c *DeleteCmd) Help() string {
	return `
Delete a cluster maintainer.

You will be prompted to confirm deletion unless --yes is set.

Examples:
  # Delete a maintainer (with confirmation prompt)
  $ bk maintainer delete my-cluster-uuid maintainer-id

  # Delete without confirmation
  $ bk maintainer delete my-cluster-uuid maintainer-id --yes

  # Use list to find maintainer IDs
  $ bk maintainer list my-cluster-uuid
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

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Are you sure you want to delete maintainer %s?", c.MaintainerID))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Deletion cancelled.")
		return nil
	}

	if err = bkIO.SpinWhile(f, "Deleting cluster maintainer", func() error {
		_, apiErr := f.RestAPIClient.ClusterMaintainers.Delete(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.MaintainerID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error deleting cluster maintainer: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Maintainer deleted successfully.")
	return nil
}
