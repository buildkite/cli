package cluster

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"golang.org/x/sync/errgroup"
)

func QueryCluster(ctx context.Context, OrganizationSlug string, ClusterID string, f *factory.Factory) (*Cluster, error) {
	q, err := graphql.GetClusterQueues(ctx, f.GraphQLClient, OrganizationSlug, ClusterID)
	if err != nil {
		return nil, fmt.Errorf("unable to read Cluster Queues: %s", err.Error())
	}

	ClusterDescription := q.Organization.Cluster.Description
	cluster := Cluster{
		OrganizationSlug: OrganizationSlug,
		ClusterID:        ClusterID,
		Name:             q.Organization.Cluster.Name,
		Description:      string(*ClusterDescription),
		Queues:           make([]Queue, len(q.Organization.Cluster.Queues.Edges)),
	}

	eg, ctx := errgroup.WithContext(ctx)

	for i, edge := range q.Organization.Cluster.Queues.Edges {

		i, edge := i, edge
		eg.Go(func() error {
			agent, err := graphql.GetClusterQueueAgent(ctx, f.GraphQLClient, OrganizationSlug, []string{edge.Node.Id})
			if err != nil {
				return fmt.Errorf("unable to read Cluster Queue Agents %s: %s", edge.Node.Id, err.Error())
			}

			cluster.Queues[i] = Queue{
				Id:           edge.Node.Id,
				Name:         edge.Node.Key,
				ActiveAgents: len(agent.Organization.Agents.Edges),
			}

			return nil
		})

	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &cluster, nil
}
