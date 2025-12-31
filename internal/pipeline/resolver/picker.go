package resolver

import (
	"slices"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

// PipelinePicker is a function used to pick a pipeline from a list.
//
// It is indended to be used from pipeline resolvers that resolve multiple pipelines.
type PipelinePicker func([]pipeline.Pipeline) *pipeline.Pipeline

func PassthruPicker(p []pipeline.Pipeline) *pipeline.Pipeline {
	return &p[0]
}

// PickOneWithFactory returns a picker that uses the factory's NoInput flag.
// When multiple pipelines are found and NoInput is true, it fails instead of prompting.
func PickOneWithFactory(f *factory.Factory) PipelinePicker {
	return func(pipelines []pipeline.Pipeline) *pipeline.Pipeline {
		if len(pipelines) == 0 {
			return nil
		}

		// no need to prompt for only one option
		if len(pipelines) == 1 {
			return &pipelines[0]
		}

		names := make([]string, len(pipelines))
		for i, p := range pipelines {
			names[i] = p.Name
		}

		chosen, err := io.PromptForOne("pipeline", names, f.NoInput)
		if err != nil {
			return nil
		}

		// Find which pipeline was chosen
		index := slices.IndexFunc(pipelines, func(p pipeline.Pipeline) bool {
			return p.Name == chosen
		})

		if index < 0 {
			// Shouldn't happen, just in case
			return nil
		}

		return &pipelines[index]
	}
}

// CachedPicker returns a PipelinePicker that saves the given pipelines to local config as well as running the provider
// picker.
func CachedPicker(conf *config.Config, picker PipelinePicker, inGitRepo bool) PipelinePicker {
	return func(pipelines []pipeline.Pipeline) *pipeline.Pipeline {
		// run the picker first because we want to put the chosen on at the top of the saved list
		chosen := picker(pipelines)
		// if chosen is nil, either there were no pipelines to begin with, or the user cancelled the picker, so we
		// probably shouldnt save them to config
		if chosen == nil {
			return nil
		}

		// pointers and slices are getting in our way here, so copy the current pipeline pointed to by chosen into a
		// temporary variable to later return, as the value chosen points to is going to change when we rearrange the
		// pipelines slice
		tmp := *chosen
		index := slices.IndexFunc(pipelines, func(p pipeline.Pipeline) bool {
			return tmp.Name == p.Name
		})
		pipelines[0], pipelines[index] = tmp, pipelines[0]

		// save the pipelines to local config before passing to the picker
		err := conf.SetPreferredPipelines(pipelines, inGitRepo)
		if err != nil {
			return nil
		}

		return &tmp
	}
}
