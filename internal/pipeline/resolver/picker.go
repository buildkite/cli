package resolver

import (
	"sort"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

// PipelinePicker is a function used to pick a pipeline from a list.
//
// It is indended to be used from pipeline resolvers that resolve multiple pipelines.
type PipelinePicker func([]pipeline.Pipeline) *pipeline.Pipeline

func PassthruPicker(p []pipeline.Pipeline) *pipeline.Pipeline {
	return &p[0]
}

// CachedPicker returns a PipelinePicker that saves the given pipelines to local config as well as running the provider
// picker.
func CachedPicker(conf *config.Config, picker PipelinePicker) PipelinePicker {
	return func(pipelines []pipeline.Pipeline) *pipeline.Pipeline {
		// run the picker first because we want to put the chosen on at the top of the saved list
		chosen := picker(pipelines)
		// if chosen is nil, either there were no pipelines to begin with, or the user cancelled the picker, so we
		// probably shouldnt save them to config
		if chosen == nil {
			return nil
		}

		// swap the chosen pipeline with the first element
		index := sort.Search(len(pipelines), func(i int) bool {
			return chosen == &pipelines[i]
		})
		pipelines[0], pipelines[index] = *chosen, pipelines[0]

		// save the pipelines to local config before passing to the picker
		err := conf.SetPreferredPipelines(pipelines)
		if err != nil {
			return nil
		}

		return chosen
	}
}
