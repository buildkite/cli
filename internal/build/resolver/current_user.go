package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/build"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
)

// ResolveBuildForCurrentUser Finds the most recent build for the current user and branch
func ResolveBuildForCurrentUser(branch string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {
		var user *buildkite.User

		user, _, err := f.RestAPIClient.User.Get()
		if err != nil {
			return nil, err
		}

		return ResolveBuildForUser(ctx, *user.ID, branch, pipelineResolver, f)
	}
}
