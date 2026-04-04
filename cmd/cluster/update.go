package cluster

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
	ClusterUUID      string `arg:"" help:"Cluster UUID to update" name:"cluster-uuid"`
	Name             string `help:"New name for the cluster" optional:""`
	Description      string `help:"New description for the cluster" optional:""`
	Emoji            string `help:"New emoji for the cluster (e.g. :rocket:)" optional:""`
	Color            string `help:"New color hex code for the cluster (e.g. #FF0000)" optional:""`
	DefaultQueueUUID string `help:"UUID of the queue to set as the default" optional:"" name:"default-queue-uuid" aliases:"default-queue-id"`
	output.OutputFlags
}

func (c *UpdateCmd) Help() string {
	return `
Update a cluster's settings.

At least one of --name, --description, --emoji, --color, or --default-queue-uuid must be provided.

Examples:
  # Update a cluster's name
  $ bk cluster update my-cluster-uuid --name "New Name"

  # Update description and color
  $ bk cluster update my-cluster-uuid --description "Updated description" --color "#00FF00"

  # Set the default queue
  $ bk cluster update my-cluster-uuid --default-queue-uuid my-queue-uuid

  # Output the updated cluster as JSON
  $ bk cluster update my-cluster-uuid --name "New Name" -o json
`
}

func (c *UpdateCmd) Validate() error {
	if c.Name == "" && c.Description == "" && c.Emoji == "" && c.Color == "" && c.DefaultQueueUUID == "" {
		return fmt.Errorf("at least one of --name, --description, --emoji, --color, or --default-queue-uuid must be provided")
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	input := buildkite.ClusterUpdate{
		Name:           c.Name,
		Description:    c.Description,
		Emoji:          c.Emoji,
		Color:          c.Color,
		DefaultQueueID: c.DefaultQueueUUID,
	}

	var cluster buildkite.Cluster
	if err = bkIO.SpinWhile(f, "Updating cluster", func() error {
		var apiErr error
		cluster, _, apiErr = f.RestAPIClient.Clusters.Update(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error updating cluster: %v", err)
	}

	clusterView := output.Viewable[buildkite.Cluster]{
		Data:   cluster,
		Render: renderClusterText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, clusterView, format)
	}

	fmt.Fprintf(os.Stdout, "Cluster %s updated successfully\n\n", cluster.Name)
	return output.Write(os.Stdout, clusterView, format)
}
