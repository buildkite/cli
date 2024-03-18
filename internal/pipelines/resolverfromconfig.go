package pipelines

import (
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func ResolveFromConfig(f *factory.Factory) ([]string, error) {

	var localPipelines []string
	// check if there is a local config file
	err := f.LocalConfig.Read()
	if err != nil {
		fmt.Printf("Error reading local config: %s", err)
		return nil, err
	}
	// if there is a pipeline defined in the local config, return it
	if len(f.LocalConfig.Pipeline) > 0 {
		localPipelines = append(localPipelines, f.LocalConfig.Pipeline)
	}
	return localPipelines, nil
}
