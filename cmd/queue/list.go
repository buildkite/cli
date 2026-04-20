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

type ListCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID to list queues for" name:"cluster-uuid"`
	PerPage     int    `help:"Number of queues per page" default:"30"`
	Limit       int    `help:"Maximum number of queues to return" default:"100"`
	output.OutputFlags
}

func (c *ListCmd) Validate() error {
	if c.PerPage < 1 {
		return fmt.Errorf("invalid --per-page %d: must be greater than 0", c.PerPage)
	}

	if c.Limit < 0 {
		return fmt.Errorf("invalid --limit %d: must be greater than or equal to 0", c.Limit)
	}

	return nil
}

func (c *ListCmd) Help() string {
	return `
List cluster queues.

Examples:
  # List all queues for a cluster
  $ bk queue list my-cluster-uuid

  # Return more queues
  $ bk queue list my-cluster-uuid --limit 200

  # List in JSON format
  $ bk queue list my-cluster-uuid -o json
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	var queues []buildkite.ClusterQueue
	page := 1
	var previousFirstQueueID string

	for len(queues) < c.Limit {
		opts := &buildkite.ClusterQueuesListOptions{
			ListOptions: buildkite.ListOptions{
				Page:    page,
				PerPage: c.PerPage,
			},
		}

		var pageQueues []buildkite.ClusterQueue
		if err := bkIO.SpinWhile(f, "Fetching cluster queues", func() error {
			var apiErr error
			pageQueues, _, apiErr = f.RestAPIClient.ClusterQueues.List(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, opts)
			return apiErr
		}); err != nil {
			return fmt.Errorf("error fetching cluster queues: %w", err)
		}

		if len(pageQueues) == 0 {
			break
		}

		if page > 1 && pageQueues[0].ID == previousFirstQueueID {
			return fmt.Errorf("API returned duplicate page content at page %d, stopping pagination to prevent infinite loop", page)
		}
		previousFirstQueueID = pageQueues[0].ID

		queues = append(queues, pageQueues...)

		if len(pageQueues) < c.PerPage {
			break
		}

		if len(queues) >= c.Limit {
			break
		}

		page++
	}

	if len(queues) > c.Limit {
		queues = queues[:c.Limit]
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, queues, format)
	}

	if len(queues) == 0 {
		fmt.Fprintln(os.Stdout, "No queues found")
		return nil
	}

	rows := make([][]string, 0, len(queues))
	for _, q := range queues {
		paused := "No"
		if q.DispatchPaused {
			paused = "Yes"
		}
		rows = append(rows, []string{q.Key, output.ValueOrDash(q.Description), paused, q.ID})
	}

	table := output.Table(
		[]string{"Key", "Description", "Paused", "ID"},
		rows,
		map[string]string{"key": "bold"},
	)

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	_, err = fmt.Fprintf(writer, "Queues (%d)\n\n%s\n", len(queues), table)
	return err
}
