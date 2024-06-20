package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/build"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func ResolveBuildByUserId(uuid string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {

		return ResolveBuildForUser(ctx, uuid, "", pipelineResolver, f)
	}
}
