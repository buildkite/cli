package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/cluster"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
)

func NewCmdClusterList(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Short:                 "List all clusters.",
		Long: heredoc.Doc(`
      List the clusters for an organization.
    `),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusters, err := listClusters(cmd.Context(), f)
			if err != nil {
				return err
			}

			summary := cluster.ClusterViewTable(clusters...)
			fmt.Fprintf(os.Stdout, "%v\n", summary)

			return nil
		},
	}

	return &cmd
}

func listClusters(ctx context.Context, f *factory.Factory) ([]buildkite.Cluster, error) {
	clusters, _, err := f.RestAPIClient.Clusters.List(ctx, f.Config.OrganizationSlug(), nil)
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
