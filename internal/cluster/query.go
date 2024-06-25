package cluster

import (
	"context"
	"fmt"
	"sync"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func QueryCluster(ctx context.Context, OrganizationSlug string, ClusterID string, f *factory.Factory) (*Cluster, error) {

	q, err := graphql.GetClusterQueues(ctx, f.GraphQLClient, OrganizationSlug, ClusterID)

	if err != nil {
		fmt.Println("Unable to read Cluster Queues: ", err.Error())
		return nil, err
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
	errorChannel := make(chan error, len(q.Organization.Cluster.Queues.Edges))
	var wg sync.WaitGroup

	for _, edge := range q.Organization.Cluster.Queues.Edges {
		wg.Add(1)
		go func(id, key string) {
			defer wg.Done()
			agent, err := graphql.GetClusterQueueAgent(ctx, f.GraphQLClient, OrganizationSlug, []string{id})
			if err != nil {
				errorChannel <- fmt.Errorf("unable to read Cluster Queue Agents %s: %s", id, err.Error())
				return
			}

			queue := Queue{
				Id:           id,
				Name:         key,
				ActiveAgents: len(agent.Organization.Agents.Edges),
			}
			queueChannel <- queue
		}(edge.Node.Id, edge.Node.Key)

	}

	go func() {
		wg.Wait()
		close(queueChannel)
		close(errorChannel)
	}()

	for queue := range queueChannel {
		cluster.Queues = append(cluster.Queues, queue)

	}
	return &cluster, nil
}
