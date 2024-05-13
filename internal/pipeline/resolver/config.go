package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromConfig(conf *config.Config, picker PipelinePicker) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {
		pipelines := conf.PreferredPipelines()

		if len(pipelines) == 0 {
			return nil, nil
		}

		return picker(pipelines), nil
	}
}
