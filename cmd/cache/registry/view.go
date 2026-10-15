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
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type ViewCmd struct {
	ClusterUUID  string `arg:"" help:"Cluster UUID the cache registry belongs to" name:"cluster-uuid"`
	RegistryUUID string `arg:"" help:"Cache registry UUID to view" name:"registry-uuid"`
	output.OutputFlags
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	writer, cleanup := commandWriter(f, format)
	defer func() { _ = cleanup() }()
	return c.run(ctx, f, f.Config.OrganizationSlug(), writer, format)
}

func (c *ViewCmd) run(ctx context.Context, f *factory.Factory, org string, writer io.Writer, format output.Format) error {
	var registry buildkite.CacheRegistry
	if err := bkIO.SpinWhile(f, "Loading cache registry", func() error {
		var apiErr error
		registry, _, apiErr = f.RestAPIClient.CacheRegistries.Get(ctx, org, c.ClusterUUID, c.RegistryUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error loading cache registry: %w", err)
	}

	return output.Write(writer, output.Viewable[buildkite.CacheRegistry]{Data: registry, Render: renderRegistryText}, format)
}
