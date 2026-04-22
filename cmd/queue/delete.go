package queue

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
	ClusterUUID string `arg:"" help:"Cluster UUID the queue belongs to" name:"cluster-uuid"`
	QueueUUID   string `arg:"" help:"Queue UUID to delete" name:"queue-uuid"`
}

func (c *DeleteCmd) Help() string {
	return `
You will be prompted to confirm deletion unless --yes is set.

Examples:
  # Delete a queue
  $ bk queue delete my-cluster-uuid my-queue-uuid

  # Delete a queue without confirmation
  $ bk queue delete my-cluster-uuid my-queue-uuid --yes
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

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Are you sure you want to delete queue %s?", c.QueueUUID))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Deletion cancelled.")
		return nil
	}

	if err = bkIO.SpinWhile(f, "Deleting cluster queue", func() error {
		_, apiErr := f.RestAPIClient.ClusterQueues.Delete(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error deleting cluster queue: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Queue %s deleted successfully.\n", c.QueueUUID)
	return nil
}
