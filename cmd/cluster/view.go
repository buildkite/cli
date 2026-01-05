package cluster

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
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

type ViewCmd struct {
	ClusterID string `arg:"" help:"Cluster ID to view"`
	Output    string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:"json,yaml,text"`
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
	f.NoPager = f.NoPager || globals.DisablePager()

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

	if format != output.FormatText {
		return output.Write(os.Stdout, clusterView, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	return output.Write(writer, clusterView, format)
}

func renderClusterText(c buildkite.Cluster) string {
	rows := [][]string{
		{"Description", output.ValueOrDash(c.Description)},
		{"Color", output.ValueOrDash(c.Color)},
		{"Emoji", output.ValueOrDash(c.Emoji)},
		{"ID", output.ValueOrDash(c.ID)},
		{"GraphQL ID", output.ValueOrDash(c.GraphQLID)},
		{"Default Queue ID", output.ValueOrDash(c.DefaultQueueID)},
		{"Web URL", output.ValueOrDash(c.WebURL)},
		{"API URL", output.ValueOrDash(c.URL)},
		{"Queues URL", output.ValueOrDash(c.QueuesURL)},
		{"Queue URL", output.ValueOrDash(c.DefaultQueueURL)},
	}

	if c.CreatedBy.ID != "" {
		rows = append(rows,
			[]string{"Created By Name", output.ValueOrDash(c.CreatedBy.Name)},
			[]string{"Created By Email", output.ValueOrDash(c.CreatedBy.Email)},
			[]string{"Created By ID", output.ValueOrDash(c.CreatedBy.ID)},
		)
	}

	if c.CreatedAt != nil {
		rows = append(rows, []string{"Created At", c.CreatedAt.Format(time.RFC3339)})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Viewing %s\n\n", output.ValueOrDash(c.Name))

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)
	return sb.String()
}
