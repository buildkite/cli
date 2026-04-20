package queue

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
	ClusterUUID string `arg:"" help:"Cluster UUID the queue belongs to" name:"cluster-uuid"`
	QueueUUID   string `arg:"" help:"Queue UUID to view" name:"queue-uuid"`
	output.OutputFlags
}

func (c *ViewCmd) Help() string {
	return `
Examples:
  # View a queue
  $ bk queue view my-cluster-uuid my-queue-uuid

  # View a queue in JSON format
  $ bk queue view my-cluster-uuid my-queue-uuid -o json
`
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var queue buildkite.ClusterQueue
	if err = bkIO.SpinWhile(f, "Loading queue information", func() error {
		var apiErr error
		queue, _, apiErr = f.RestAPIClient.ClusterQueues.Get(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, c.QueueUUID)
		return apiErr
	}); err != nil {
		return err
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

	return output.Write(writer, queueView, format)
}

func renderQueueText(q buildkite.ClusterQueue) string {
	paused := "No"
	if q.DispatchPaused {
		paused = "Yes"
	}

	rows := [][]string{
		{"Key", output.ValueOrDash(q.Key)},
		{"Description", output.ValueOrDash(q.Description)},
		{"ID", output.ValueOrDash(q.ID)},
		{"GraphQL ID", output.ValueOrDash(q.GraphQLID)},
		{"Retry Agent Affinity", output.ValueOrDash(string(q.RetryAgentAffinity))},
		{"Dispatch Paused", paused},
		{"Web URL", output.ValueOrDash(q.WebURL)},
		{"API URL", output.ValueOrDash(q.URL)},
		{"Cluster URL", output.ValueOrDash(q.ClusterURL)},
	}

	if q.DispatchPaused {
		rows = append(rows, []string{"Dispatch Paused Note", output.ValueOrDash(q.DispatchPausedNote)})
		if q.DispatchPausedAt != nil {
			rows = append(rows, []string{"Dispatch Paused At", q.DispatchPausedAt.Format(time.RFC3339)})
		}
		if q.DispatchPausedBy != nil {
			rows = append(rows,
				[]string{"Dispatch Paused By Name", output.ValueOrDash(q.DispatchPausedBy.Name)},
				[]string{"Dispatch Paused By Email", output.ValueOrDash(q.DispatchPausedBy.Email)},
			)
		}
	}

	if q.CreatedBy.ID != "" {
		rows = append(rows,
			[]string{"Created By Name", output.ValueOrDash(q.CreatedBy.Name)},
			[]string{"Created By Email", output.ValueOrDash(q.CreatedBy.Email)},
			[]string{"Created By ID", output.ValueOrDash(q.CreatedBy.ID)},
		)
	}

	if q.CreatedAt != nil {
		rows = append(rows, []string{"Created At", q.CreatedAt.Format(time.RFC3339)})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Queue: %s\n\n", output.ValueOrDash(q.Key))

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)
	return sb.String()
}
