package pipelines

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func ResolveFromConfig(f *factory.Factory) string {

	defaultPipeline := "" // default to empty string

	// check if there is a local config file
	err := f.LocalConfig.Read()
	if err == nil && f.LocalConfig.Pipeline != "" {
		defaultPipeline = f.LocalConfig.Pipeline
	}

	return defaultPipeline
}
