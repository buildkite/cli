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

type CreateCmd struct {
	Name        string `help:"The name of the cluster" required:""`
	Description string `help:"A description of the cluster" optional:""`
	Emoji       string `help:"An emoji for the cluster (e.g. :rocket:)" optional:""`
	Color       string `help:"A color hex code for the cluster (e.g. #FF0000)" optional:""`
	output.OutputFlags
}

func (c *CreateCmd) Help() string {
	return `
Create a new cluster in the organization.

Examples:
  # Create a cluster with just a name
  $ bk cluster create --name "My Cluster"

  # Create a cluster with all fields
  $ bk cluster create --name "My Cluster" --description "Runs production workloads" --emoji :rocket: --color "#FF0000"

  # Create a cluster and output as JSON
  $ bk cluster create --name "My Cluster" -o json
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	input := buildkite.ClusterCreate{
		Name:        c.Name,
		Description: c.Description,
		Emoji:       c.Emoji,
		Color:       c.Color,
	}

	var cluster buildkite.Cluster
	spinErr := bkIO.SpinWhile(f, "Creating cluster", func() {
		cluster, _, err = f.RestAPIClient.Clusters.Create(ctx, f.Config.OrganizationSlug(), input)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error creating cluster: %v", err)
	}

	clusterView := output.Viewable[buildkite.Cluster]{
		Data:   cluster,
		Render: renderClusterText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, clusterView, format)
	}

	fmt.Fprintf(os.Stdout, "Cluster %s created successfully\n\n", cluster.Name)
	return output.Write(os.Stdout, clusterView, format)
}
