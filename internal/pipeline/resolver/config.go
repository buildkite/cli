package resolver

import (
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromConfig(c *config.LocalConfig) PipelineResolverFn {
	return func() (*pipeline.Pipeline, error) {

		var pipelines []string
		var defaultPipeline string

		defaultPipeline = c.DefaultPipeline
		if defaultPipeline == "" && len(c.Pipelines) == 0 {
			return nil, nil
		}

		if defaultPipeline == "" && len(c.Pipelines) >= 1 {
			defaultPipeline = c.Pipelines[0]
		}

		defaultExists := false
		for _, opt := range c.Pipelines {
			if defaultPipeline == opt {
				defaultExists = true
			}
			pipelines = append(pipelines, opt)
		}

		if !defaultExists { //add default pipeline to the list of pipelines
			pipelines = append([]string{defaultPipeline}, pipelines...)
		}

		selected, err := pipeline.RenderOptions(defaultPipeline, pipelines)
		if err != nil {
			return nil, err
		}

		return &pipeline.Pipeline{
			Name: selected,
			Org:  c.Organization,
		}, nil
	}
}
