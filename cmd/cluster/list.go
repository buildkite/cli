package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/cluster"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ListCmd struct {
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ListCmd) Help() string {
	return `
List the clusters for an organization.

Examples:
  # List all clusters
  $ bk cluster list

  # List clusters in JSON format
  $ bk cluster list -o json
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	clusters, err := listClusters(ctx, f)
	if err != nil {
		return err
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, clusters, format)
	}

	summary := cluster.ClusterViewTable(clusters...)
	fmt.Fprintf(os.Stdout, "%v\n", summary)

	return nil
}

func listClusters(ctx context.Context, f *factory.Factory) ([]buildkite.Cluster, error) {
	var clusters []buildkite.Cluster
	var err error

	spinErr := bkIO.SpinWhile(f, "Loading clusters information", func() {
		clusters, _, err = f.RestAPIClient.Clusters.List(ctx, f.Config.OrganizationSlug(), nil)
	})
	if spinErr != nil {
		return nil, spinErr
	}
	if err != nil {
		return nil, fmt.Errorf("error fetching cluster list: %v", err)
	}

	if len(clusters) < 1 {
		return nil, errors.New("no clusters found in organization")
	}

	clusterList := make([]buildkite.Cluster, len(clusters))
	var wg sync.WaitGroup
	errChan := make(chan error, len(clusters))
	for i, c := range clusters {
		wg.Add(1)
		go func(i int, c buildkite.Cluster) {
			defer wg.Done()
			clusterList[i] = buildkite.Cluster{
				Color:           c.Color,
				CreatedAt:       c.CreatedAt,
				CreatedBy:       c.CreatedBy,
				DefaultQueueID:  c.DefaultQueueID,
				DefaultQueueURL: c.DefaultQueueURL,
				Description:     c.Description,
				Emoji:           c.Emoji,
				GraphQLID:       c.GraphQLID,
				ID:              c.ID,
				Name:            c.Name,
				QueuesURL:       c.QueuesURL,
				URL:             c.URL,
				WebURL:          c.WebURL,
			}
		}(i, c)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return clusterList, nil
}
