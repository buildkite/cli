package cluster

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func renderClusterText(c buildkite.Cluster) string {
	var b bytes.Buffer

	section := func(name string) {
		fmt.Fprintf(&b, "\n%s\n", name)
	}
	field := func(name, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", name+":", value)
	}

	// Basic Information
	section("Cluster Details")
	field("Name", c.Name)
	if c.Emoji != "" {
		field("Emoji", c.Emoji)
	}
	if c.Description != "" {
		field("Description", c.Description)
	}
	if c.Color != "" {
		field("Color", c.Color)
	}

	// IDs and URLs
	section("Identifiers")
	field("ID", c.ID)
	field("GraphQL ID", c.GraphQLID)
	field("Default Queue ID", c.DefaultQueueID)

	// URLs
	section("URLs")
	field("Web URL", c.WebURL)
	field("API URL", c.URL)
	field("Queues URL", c.QueuesURL)
	field("Queue URL", c.DefaultQueueURL)

	// Creator Information
	if c.CreatedBy.ID != "" {
		section("Created By")
		field("Name", c.CreatedBy.Name)
		field("Email", c.CreatedBy.Email)
		field("ID", c.CreatedBy.ID)
		if c.CreatedAt != nil {
			field("Created At", c.CreatedAt.Format(time.RFC3339))
		}
	}

	return b.String()
}

type ViewCmd struct {
	ClusterID string `arg:"" help:"Cluster ID to view"`
	Output    string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ViewCmd) Help() string {
	return `
It accepts cluster id.

Examples:
  # View a cluster
  $ bk cluster view my-cluster-id

  # View cluster in JSON format
  $ bk cluster view my-cluster-id -o json
`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	format := output.Format(c.Output)
	if format != output.FormatJSON && format != output.FormatYAML && format != output.FormatText {
		return fmt.Errorf("invalid output format: %s", c.Output)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var cluster buildkite.Cluster
	spinErr := bkIO.SpinWhile(f, "Loading cluster information", func() {
		cluster, _, err = f.RestAPIClient.Clusters.Get(ctx, f.Config.OrganizationSlug(), c.ClusterID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	clusterView := output.Viewable[buildkite.Cluster]{
		Data:   cluster,
		Render: renderClusterText,
	}

	return output.Write(os.Stdout, clusterView, format)
}
