package resolver

import (
	"github.com/buildkite/cli/v3/internal/pipeline"
)

type PipelinePicker func([]pipeline.Pipeline) *pipeline.Pipeline

func PassthruPicker(p []pipeline.Pipeline) *pipeline.Pipeline {
	return &p[0]
}
