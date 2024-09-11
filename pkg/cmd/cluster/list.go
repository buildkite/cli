package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/cluster"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
			var listOptions buildkite.ClustersListOptions

			clusters, err := listClusters(cmd.Context(), &listOptions, f)
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

func listClusters(ctx context.Context, lo *buildkite.ClustersListOptions, f *factory.Factory) ([]cluster.Cluster, error) {
	clusters, _, err := f.RestAPIClient.Clusters.List(f.Config.OrganizationSlug(), lo)
	if err != nil {
		return nil, fmt.Errorf("error fetching cluster list: %v", err)
	}

	if len(clusters) < 1 {
		return nil, errors.New("no clusters found in organization")
	}

	eg, ctx := errgroup.WithContext(ctx)
	clusterList := make([]cluster.Cluster, len(clusters))
	for i, c := range clusters {
		cQs, err := cluster.GetQueues(ctx, f, f.Config.OrganizationSlug(), *c.ID, (*buildkite.ClusterQueuesListOptions)(lo))
		if err != nil {
			return nil, err
		}
		i, c := i, c
		eg.Go(func() error {
			clusterList[i] = cluster.Cluster{
				Color:           c.Color,
				CreatedAt:       *c.CreatedAt,
				CreatedBy:       *c.CreatedBy,
				DefaultQueueID:  c.DefaultQueueID,
				DefaultQueueURL: c.DefaultQueueURL,
				Description:     c.Description,
				Emoji:           c.Emoji,
				GraphQLID:       *c.GraphQLID,
				ID:              *c.ID,
				Name:            *c.Name,
				Queues:          cQs,
				QueuesURL:       *c.QueuesURL,
				URL:             *c.URL,
				WebURL:          *c.WebURL,
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return clusterList, nil
}
