package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func ResolveBuildWithOpts(f *factory.Factory, pipelineResolver pipelineResolver.PipelineResolverFn, listOpts ...options.OptionsFn) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {
		pipeline, err := pipelineResolver(ctx)
		if err != nil {
			return nil, err
		}
		if pipeline == nil {
			return nil, fmt.Errorf("failed to resolve a pipeline to query builds on")
		}

		opts := &buildkite.BuildsListOptions{
			ListOptions: buildkite.ListOptions{
				PerPage: 1,
			},
		}
		for _, opt := range listOpts {
			err = opt(opts)
			if err != nil {
				return nil, err
			}
		}

		builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, f.Config.OrganizationSlug(), pipeline.Name, opts)
		if err != nil {
			return nil, err
		}
		if len(builds) == 0 {
			return nil, nil
		}

		return &build.Build{
			Organization: f.Config.OrganizationSlug(),
			Pipeline:     pipeline.Name,
			BuildNumber:  builds[0].Number,
		}, nil
	}
}
