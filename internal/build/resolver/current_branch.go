package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/graphql"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/go-git/go-git/v5"
)

// ResolveBuildFromCurrentBranch Finds the most recent build for the branch in the current working directory
func ResolveBuildFromCurrentBranch(repo *git.Repository, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	// there is nothing to resolve from if we aren't in a git repository, so short circuit that
	if repo == nil {
		return func(ctx context.Context) (*build.Build, error) {
			return nil, nil
		}
	}
	return func(ctx context.Context) (*build.Build, error) {
		// find the pipeline first, then we can make a graphql query to find the most recent build for that branch
		pipeline, err := pipelineResolver(ctx)
		if err != nil {
			return nil, err
		}
		if pipeline == nil {
			return nil, fmt.Errorf("Failed to resolve a pipeline to query builds on.")
		}

		head, err := repo.Head()
		if err != nil {
			return nil, err
		}
		branch := head.Name().Short()

		b, err := graphql.RecentBuildsForBranch(ctx, f.GraphQLClient, branch, fmt.Sprintf("%s/%s", pipeline.Org, pipeline.Name))
		if err != nil {
			return nil, err
		}
		if len(b.Pipeline.Builds.Edges) < 1 {
			return nil, fmt.Errorf("No builds found for pipeline %s, branch %s", pipeline.Name, branch)
		}

		node := b.Pipeline.Builds.Edges[0].Node
		return &build.Build{
			BuildNumber:  node.Number,
			Organization: pipeline.Org,
			Pipeline:     pipeline.Name,
		}, nil
	}
}
