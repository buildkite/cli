package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromConfig(conf *config.Config) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {
		preferredPipelines := conf.PreferredPipelines()

		if len(preferredPipelines) == 0 {
			return nil, nil
		}

		return &preferredPipelines[0], nil
	}
}
