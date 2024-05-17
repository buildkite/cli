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
		// find the pipeline first, then we can make a graphql query to find the most recent build for that branch
		var pipeline *pipeline.Pipeline
		var user *buildkite.User

		// use an errgroup so a few API calls can be done in parallel
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
			return nil, fmt.Errorf("Failed to resolve a pipeline to query builds on.")
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
			// TODO should this return an error since it didnt find any builds?
			return nil, nil
		}
		return &build.Build{
			Organization: f.Config.OrganizationSlug(),
			Pipeline:     pipeline.Name,
			BuildNumber:  *builds[0].Number,
		}, nil
	}
}
