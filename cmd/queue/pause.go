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

type PauseCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID the queue belongs to" name:"cluster-uuid"`
	QueueUUID   string `arg:"" help:"Queue UUID to pause" name:"queue-uuid"`
	Note        string `help:"Optional note explaining why the queue is being paused" optional:"" name:"note"`
	output.OutputFlags
}

func (c *PauseCmd) Help() string {
	return `
The queue remains paused until it is resumed with "bk queue resume".

Examples:
  # Pause a queue
  $ bk queue pause my-cluster-uuid my-queue-uuid

  # Pause a queue with a note
  $ bk queue pause my-cluster-uuid my-queue-uuid --note "Pausing for maintenance"

  # Output the paused queue as JSON
  $ bk queue pause my-cluster-uuid my-queue-uuid --note "Maintenance" -o json
`
}

func (c *PauseCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	input := buildkite.ClusterQueuePause{
		Note: c.Note,
	}

	var queue buildkite.ClusterQueue
	if err = bkIO.SpinWhile(f, "Pausing cluster queue", func() error {
		var apiErr error
		queue, _, apiErr = f.RestAPIClient.ClusterQueues.Pause(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error pausing cluster queue: %w", err)
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

	queueIdentifier := queue.Key
	if queueIdentifier == "" {
		queueIdentifier = c.QueueUUID
	}

	fmt.Fprintf(writer, "Queue %s paused successfully\n\n", queueIdentifier)
	return output.Write(writer, queueView, format)
}
