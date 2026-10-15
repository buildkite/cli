package registry

import (
	"context"
	"fmt"
	"io"
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
	ClusterUUID  string `arg:"" help:"Cluster UUID the cache registry belongs to" name:"cluster-uuid"`
	RegistryUUID string `arg:"" help:"Cache registry UUID to delete" name:"registry-uuid"`
}

func (c *DeleteCmd) Help() string {
	return `
You will be prompted to confirm deletion unless --yes is set.

Examples:
  $ bk cache registry delete my-cluster-uuid my-registry-uuid
  $ bk cache registry delete my-cluster-uuid my-registry-uuid --yes
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
	return c.run(ctx, f, f.Config.OrganizationSlug(), os.Stderr)
}

func (c *DeleteCmd) run(ctx context.Context, f *factory.Factory, org string, writer io.Writer) error {
	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Are you sure you want to delete cache registry %s?", c.RegistryUUID))
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(writer, "Deletion cancelled.")
		return err
	}

	if err := bkIO.SpinWhile(f, "Deleting cache registry", func() error {
		_, apiErr := f.RestAPIClient.CacheRegistries.Delete(ctx, org, c.ClusterUUID, c.RegistryUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error deleting cache registry: %w", err)
	}
	_, err = fmt.Fprintf(writer, "Cache registry %s deleted successfully.\n", c.RegistryUUID)
	return err
}
