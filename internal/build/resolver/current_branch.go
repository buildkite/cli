package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/graphql"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

// ResolveBuildFromCurrentBranch Finds the most recent build for the branch in the current working directory
func ResolveBuildFromCurrentBranch(branch string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {
		// find the pipeline first, then we can make a graphql query to find the most recent build for that branch
		pipeline, err := pipelineResolver(ctx)
		if err != nil {
			return nil, err
		}
		if pipeline == nil {
			return nil, fmt.Errorf("Failed to resolve a pipeline (%s) to query builds on.", pipeline.Name)
		}

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
