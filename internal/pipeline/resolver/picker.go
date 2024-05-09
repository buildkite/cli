package resolver

import (
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

type PipelinePicker func([]pipeline.Pipeline) *pipeline.Pipeline

func PassthruPicker(p []pipeline.Pipeline) *pipeline.Pipeline {
	return &p[0]
}

// CachedPicker returns a PipelinePicker that saves the given pipelines to local config as well as running the provider
// picker.
func CachedPicker(conf *config.Config, picker PipelinePicker) PipelinePicker {
	return func(pipelines []pipeline.Pipeline) *pipeline.Pipeline {
		// save the pipelines to local config before passing to the picker
		err := conf.SetPreferredPipelines(pipelines)
		if err != nil {
			return nil
		}

		return picker(pipelines)
	}
}
