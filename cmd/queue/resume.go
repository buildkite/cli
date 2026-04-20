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
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ResumeCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID the queue belongs to" name:"cluster-uuid"`
	QueueUUID   string `arg:"" help:"Queue UUID to resume" name:"queue-uuid"`
	output.OutputFlags
}

func (c *ResumeCmd) Help() string {
	return `
Resumes dispatch for a paused cluster queue.

Examples:
  # Resume a queue
  $ bk queue resume my-cluster-uuid my-queue-uuid

  # Output the resumed queue as JSON
  $ bk queue resume my-cluster-uuid my-queue-uuid -o json
`
}

func (c *ResumeCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	if err = bkIO.SpinWhile(f, "Resuming cluster queue", func() error {
		_, apiErr := f.RestAPIClient.ClusterQueues.Resume(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error resuming cluster queue: %w", err)
	}

	var queue buildkite.ClusterQueue
	if err = bkIO.SpinWhile(f, "Loading queue", func() error {
		var apiErr error
		queue, _, apiErr = f.RestAPIClient.ClusterQueues.Get(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error fetching cluster queue: %w", err)
	}

	queueView := output.Viewable[buildkite.ClusterQueue]{
		Data:   queue,
		Render: renderQueueText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, queueView, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	fmt.Fprintf(writer, "Queue %s resumed successfully\n\n", queue.Key)
	return output.Write(writer, queueView, format)
}
