package resolver

import (
	"context"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromConfig(conf *config.LocalConfig, picker PipelinePicker) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {
		var pipelines []pipeline.Pipeline
		var defaultPipeline string

		defaultPipeline = conf.DefaultPipeline
		if defaultPipeline == "" && len(conf.Pipelines) == 0 {
			return nil, nil
		}

		if defaultPipeline == "" && len(conf.Pipelines) >= 1 {
			defaultPipeline = conf.Pipelines[0]
		}

		defaultExists := false
		for _, opt := range conf.Pipelines {
			if defaultPipeline == opt {
				defaultExists = true
			}
			pipelines = append(pipelines, pipeline.Pipeline{Name: opt, Org: conf.Organization})
		}

		if !defaultExists { //add default pipeline to the list of pipelines
			pipelines = append(pipelines, pipeline.Pipeline{Name: defaultPipeline, Org: conf.Organization})
		}

		selected := picker(pipelines)
		return selected, nil
	}
}
