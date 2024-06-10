package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromFlag(flag string, conf *config.Config) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {
		// if the flag is empty, pass through
		if flag == "" {
			return nil, nil
		}
		org, name := parsePipelineArg(flag, conf)

		// if we get here, we should be able to parse the value and return an error if not
		// this is because a user has explicitly given an input value for us to use - we shoulnt ignore it on error
		if org == "" || name == "" {
			return nil, fmt.Errorf("unable to parse the input pipeline argument: \"%s\"", flag)
		}

		return &pipeline.Pipeline{Name: name, Org: org}, nil
	}
}
