package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/pipeline"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"golang.org/x/sync/errgroup"
)

// ResolveBuildFromCurrentBranch Finds the most recent build for the branch in the current working directory
func ResolveBuildForCurrentUser(branch string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {
		var pipeline *pipeline.Pipeline
		var user *buildkite.User

		// use an errgroup so a few API calls can be done in parallel
		// and then we check for any errors that occurred
		g, _ := errgroup.WithContext(ctx)
		g.Go(func() error {
			p, e := pipelineResolver(ctx)
			if p != nil {
				pipeline = p
			}
			return e
		})
		g.Go(func() error {
			u, _, e := f.RestAPIClient.User.Get()
			if u != nil {
				user = u
			}
			return e
		})
		err := g.Wait()
		if err != nil {
			return nil, err
		}
		if pipeline == nil {
			return nil, fmt.Errorf("failed to resolve a pipeline to query builds on.")
		}

		builds, _, err := f.RestAPIClient.Builds.ListByPipeline(f.Config.OrganizationSlug(), pipeline.Name, &buildkite.BuildsListOptions{
			Creator: *user.Email,
			Branch:  []string{branch},
			ListOptions: buildkite.ListOptions{
				PerPage: 1,
			},
		})
		if err != nil {
			return nil, err
		}
		if len(builds) == 0 {
			// we error here because this resolver is explicitly used so any case where it doesn't resolve a build is a
			// problem
			return nil, fmt.Errorf("failed to find a build for current user (email: %s)", *user.Email)
		}

		return &build.Build{
			Organization: f.Config.OrganizationSlug(),
			Pipeline:     pipeline.Name,
			BuildNumber:  *builds[0].Number,
		}, nil
	}
}
