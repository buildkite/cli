package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/build"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
)

func ResolveBuildFromCurrentUserId(uuid string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {

		opt := &buildkite.BuildsListOptions{
			Creator: uuid,
			ListOptions: buildkite.ListOptions{
				PerPage: 1,
			},
		}

		return ResolveBuildForUser(ctx, uuid, opt, pipelineResolver, f)
	}
}
