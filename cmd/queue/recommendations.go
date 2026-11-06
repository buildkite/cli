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
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type RecommendationsCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID to find missing hosted starter queues for" name:"cluster-uuid"`
	output.OutputFlags
}

func (c *RecommendationsCmd) Help() string {
	return `List eligible missing hosted starter queues. Requires read_clusters and
permission to manage the cluster. This does not create queues or list every
available instance shape. Use the returned key, instance shape and macOS version
with bk queue create. JSON output contains ready-to-submit API creation payloads.

Examples:
  $ bk queue recommendations my-cluster-uuid
  $ bk queue recommendations my-cluster-uuid -o json
`
}

func (c *RecommendationsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	var recommendations []buildkite.ClusterQueueCreate
	if err := bkIO.SpinWhile(f, "Fetching queue recommendations", func() error {
		var apiErr error
		recommendations, _, apiErr = f.RestAPIClient.ClusterQueues.Recommendations(ctx, f.Config.OrganizationSlug(), c.ClusterUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error fetching queue recommendations: %w", err)
	}
	if format != output.FormatText {
		return output.Write(os.Stdout, recommendations, format)
	}
	if len(recommendations) == 0 {
		fmt.Fprintln(os.Stdout, "No eligible missing hosted queues")
		return nil
	}

	rows := make([][]string, 0, len(recommendations))
	for _, recommendation := range recommendations {
		hosted := recommendation.HostedAgents
		rows = append(rows, []string{recommendation.Key, hosted.InstanceShape, output.ValueOrDash(hosted.MacOSVersion), output.ValueOrDash(hosted.XcodeVersion)})
	}
	table := output.Table([]string{"Key", "Instance Shape", "macOS Version", "Xcode Version"}, rows, map[string]string{"key": "bold"})
	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()
	_, err = fmt.Fprintf(writer, "Queue recommendations (%d)\n\n%s\n", len(recommendations), table)
	return err
}
