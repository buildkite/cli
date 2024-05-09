package resolver

import (
	"github.com/buildkite/cli/v3/internal/pipeline"
)

type PipelineChooser func([]pipeline.Pipeline) *pipeline.Pipeline
