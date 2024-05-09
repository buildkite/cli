package resolver

import (
	"github.com/buildkite/cli/v3/internal/pipeline"
)

type PipelinePicker func([]pipeline.Pipeline) *pipeline.Pipeline
