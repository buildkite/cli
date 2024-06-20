package cluster

import (
	"context"
	"fmt"

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

	for _, edge := range q.Organization.Cluster.Queues.Edges {
		agent, err := graphql.GetClusterQueueAgent(ctx, f.GraphQLClient, OrganizationSlug, []string{edge.Node.Id})
		if err != nil {
			return nil, fmt.Errorf("unable to read Cluster Queue Agents: %s", err.Error())

		}

		queue := Queue{
			Id:           edge.Node.Id,
			Name:         edge.Node.Key,
			ActiveAgents: len(agent.Organization.Agents.Edges),
		}
		cluster.Queues = append(cluster.Queues, queue)

	}
	return &cluster, nil
}
