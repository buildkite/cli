package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/build"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

// ResolveBuildForUserID Finds the most recent build for the user based on the user's UUID
func ResolveBuildForUserID(uuid string, pipelineResolver pipelineResolver.PipelineResolverFn, f *factory.Factory) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {

		return ResolveBuildForUser(ctx, uuid, "", pipelineResolver, f)
	}
}
