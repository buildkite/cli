package cluster

import (
	"context"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"golang.org/x/sync/errgroup"
)

func GetQueues(ctx context.Context, f *factory.Factory, orgSlug string, clusterID string, lo *buildkite.ClusterQueuesListOptions) ([]ClusterQueue, error) {
	queues, _, err := f.RestAPIClient.ClusterQueues.List(orgSlug, clusterID, lo)
	if err != nil {
		return nil, err
	}

	eg, ctx := errgroup.WithContext(ctx)
	queuesResponse := make([]ClusterQueue, len(queues))
	for i, q := range queues {
		i, q := i, q
		eg.Go(func() error {
			queuesResponse[i] = ClusterQueue{
				ClusterID:          *q.ID,
				CreatedAt:          *q.CreatedAt,
				CreatedBy:          *q.CreatedBy,
				Description:        q.Description,
				DispatchPaused:     *q.DispatchPaused,
				DispatchPausedAt:   q.DispatchPausedAt,
				DispatchPausedBy:   q.DispatchPausedBy,
				DispatchPausedNote: q.DispatchPausedNote,
				ID:                 *q.ID,
				Key:                *q.Key,
				URL:                *q.URL,
				WebURL:             *q.WebURL,
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return queuesResponse, nil
}

func GetQueueAgentCount(ctx context.Context, f *factory.Factory, orgSlug string, queues ...ClusterQueue) (int, error) {
	queueIDs := []string{}
	for _, q := range queues {
		queueIDs = append(queueIDs, q.ID)
	}
	agent, err := graphql.GetClusterQueueAgent(ctx, f.GraphQLClient, orgSlug, queueIDs)
	if err != nil {
		return 0, err
	}

	return len(agent.Organization.Agents.Edges), nil
}
