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

type CreateCmd struct {
	ClusterUUID        string `arg:"" help:"Cluster UUID to create the queue in" name:"cluster-uuid"`
	Key                string `help:"A unique key for the queue" required:""`
	Description        string `help:"A description of the queue" optional:""`
	RetryAgentAffinity string `help:"Retry agent affinity setting (prefer-warmest or prefer-different)" optional:"" name:"retry-agent-affinity"`
	output.OutputFlags
}

func (c *CreateCmd) Validate() error {
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

func (c *CreateCmd) Help() string {
	return `
Examples:
  # Create a queue with just a key
  $ bk queue create my-cluster-uuid --key my-queue

  # Create a queue with a description
  $ bk queue create my-cluster-uuid --key my-queue --description "My new queue"

  # Create a queue with retry agent affinity set
  $ bk queue create my-cluster-uuid --key my-queue --retry-agent-affinity prefer-different

  # Create a queue and output as JSON
  $ bk queue create my-cluster-uuid --key my-queue -o json
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

	input := buildkite.ClusterQueueCreate{
		Key:         c.Key,
		Description: c.Description,
	}
	if c.RetryAgentAffinity != "" {
		input.RetryAgentAffinity = buildkite.RetryAgentAffinity(c.RetryAgentAffinity)
	}

	var queue buildkite.ClusterQueue
	if err = bkIO.SpinWhile(f, "Creating queue", func() error {
		var apiErr error
		queue, _, apiErr = f.RestAPIClient.ClusterQueues.Create(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error creating queue: %w", err)
	}

	queueView := output.Viewable[buildkite.ClusterQueue]{
		Data:   queue,
		Render: renderQueueText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, queueView, format)
	}

	fmt.Fprintf(os.Stdout, "Queue %s created successfully\n\n", queue.Key)
	return output.Write(os.Stdout, queueView, format)
}
