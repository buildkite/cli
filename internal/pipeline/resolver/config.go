package resolver

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromConfig(c *config.Config) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {

		if c == nil {
			return nil, fmt.Errorf("could not determine config to use for pipeline resolution")
		}

		preferredPipelines := c.PreferredPipelines()

		if len(preferredPipelines) == 0 {
			return nil, nil
		}

		return &pipeline.Pipeline{
			Name: preferredPipelines[0],
			Org:  c.OrganizationSlug(),
		}, nil
	}
}
