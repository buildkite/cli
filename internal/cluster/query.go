package cluster

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
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
		Queues:           []Queue{},
	}

	queueChannel := make(chan Queue, len(q.Organization.Cluster.Queues.Edges))

	var wg sync.WaitGroup
	var e errgroup.Group

	for _, edge := range q.Organization.Cluster.Queues.Edges {
		wg.Add(1)

		e.Go(func() error {
			defer wg.Done()
			agent, err := graphql.GetClusterQueueAgent(ctx, f.GraphQLClient, OrganizationSlug, []string{edge.Node.Id})
			if err != nil {
				return fmt.Errorf("unable to read Cluster Queue Agents %s: %s", edge.Node.Id, err.Error())
			}

			queue := Queue{
				Id:           edge.Node.Id,
				Name:         edge.Node.Key,
				ActiveAgents: len(agent.Organization.Agents.Edges),
			}
			queueChannel <- queue
			return nil

		})
	}

	go func() {
		wg.Wait()
		close(queueChannel)
	}()

	for queue := range queueChannel {
		cluster.Queues = append(cluster.Queues, queue)
	}

	if err := e.Wait(); err != nil {
		return nil, fmt.Errorf("error waiting for goroutines: %s", err.Error())
	}

	return &cluster, nil
}
