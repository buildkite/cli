package cluster

import (
	"context"
	"sync"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
)

func GetQueues(ctx context.Context, f *factory.Factory, orgSlug string, clusterID string, lo *buildkite.ClusterQueuesListOptions) ([]buildkite.ClusterQueue, error) {
	queues, _, err := f.RestAPIClient.ClusterQueues.List(ctx, orgSlug, clusterID, lo)
	if err != nil {
		return nil, err
	}

	queuesResponse := make([]buildkite.ClusterQueue, len(queues))
	var wg sync.WaitGroup
	errChan := make(chan error, len(queues))
	for i, q := range queues {
		wg.Add(1)
		go func(i int, q buildkite.ClusterQueue) {
			defer wg.Done()
			queuesResponse[i] = buildkite.ClusterQueue{
				CreatedAt:          q.CreatedAt,
				CreatedBy:          q.CreatedBy,
				Description:        q.Description,
				DispatchPaused:     q.DispatchPaused,
				DispatchPausedAt:   q.DispatchPausedAt,
				DispatchPausedBy:   q.DispatchPausedBy,
				DispatchPausedNote: q.DispatchPausedNote,
				ID:                 q.ID,
				Key:                q.Key,
				URL:                q.URL,
				WebURL:             q.WebURL,
			}
		}(i, q)
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

	return queuesResponse, nil
}

func GetQueueAgentCount(ctx context.Context, f *factory.Factory, orgSlug string, queues ...buildkite.ClusterQueue) (int, error) {
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
