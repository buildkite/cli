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

type UpdateCmd struct {
	ClusterUUID        string `arg:"" help:"Cluster UUID the queue belongs to" name:"cluster-uuid"`
	QueueUUID          string `arg:"" help:"Queue UUID to update" name:"queue-uuid"`
	Description        string `help:"New description for the queue" optional:""`
	RetryAgentAffinity string `help:"Retry agent affinity (prefer-warmest or prefer-different)" optional:"" name:"retry-agent-affinity"`
	output.OutputFlags
}

func (c *UpdateCmd) Validate() error {
	if c.Description == "" && c.RetryAgentAffinity == "" {
		return fmt.Errorf("at least one of --description or --retry-agent-affinity must be provided")
	}

	switch buildkite.RetryAgentAffinity(c.RetryAgentAffinity) {
	case "", buildkite.RetryAgentAffinityPreferWarmest, buildkite.RetryAgentAffinityPreferDifferent:
		return nil
	default:
		return fmt.Errorf("invalid --retry-agent-affinity value %q: must be %s or %s",
			c.RetryAgentAffinity,
			buildkite.RetryAgentAffinityPreferWarmest,
			buildkite.RetryAgentAffinityPreferDifferent,
		)
	}
}

func (c *UpdateCmd) Help() string {
	return `
At least one of --description or --retry-agent-affinity must be provided.

Examples:
  # Update a queue's description
  $ bk queue update my-cluster-uuid my-queue-uuid --description "New description"

  # Update retry agent affinity
  $ bk queue update my-cluster-uuid my-queue-uuid --retry-agent-affinity prefer-different

  # Update both settings
  $ bk queue update my-cluster-uuid my-queue-uuid --description "New description" --retry-agent-affinity prefer-warmest

  # Output the updated queue as JSON
  $ bk queue update my-cluster-uuid my-queue-uuid --description "New description" -o json
`
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

	input := buildkite.ClusterQueueUpdate{
		Description: c.Description,
	}
	if c.RetryAgentAffinity != "" {
		input.RetryAgentAffinity = buildkite.RetryAgentAffinity(c.RetryAgentAffinity)
	}

	var queue buildkite.ClusterQueue
	if err = bkIO.SpinWhile(f, "Updating cluster queue", func() error {
		var apiErr error
		queue, _, apiErr = f.RestAPIClient.ClusterQueues.Update(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error updating cluster queue: %w", err)
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

	fmt.Fprintf(writer, "Queue %s updated successfully\n\n", queue.Key)
	return output.Write(writer, queueView, format)
}
