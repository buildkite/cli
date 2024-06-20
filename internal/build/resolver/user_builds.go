package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/build"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
)

// ResolveBuildForUser Finds the most recent build for the user and branch
func ResolveBuildForUser(ctx context.Context, userInfo string, branch string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) (*build.Build, error) {

	pipeline, err := pipelineResolver(ctx)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, fmt.Errorf("failed to resolve a pipeline to query builds on")
	}

	opt := &buildkite.BuildsListOptions{
		Creator: userInfo,
		ListOptions: buildkite.ListOptions{
			PerPage: 1,
		},
	}

	if len(branch) > 0 {
		opt.Branch = []string{branch}
	}

	builds, _, err := f.RestAPIClient.Builds.ListByPipeline(f.Config.OrganizationSlug(), pipeline.Name, opt)

	if err != nil {
		return nil, err
	}
	if len(builds) == 0 {
		// we error here because this resolver is explicitly used so any case where it doesn't resolve a build is a
		// problem
		return nil, fmt.Errorf("failed to find a build for current user (%s)", userInfo)
	}

	return &build.Build{
		Organization: f.Config.OrganizationSlug(),
		Pipeline:     pipeline.Name,
		BuildNumber:  *builds[0].Number,
	}, nil
}
