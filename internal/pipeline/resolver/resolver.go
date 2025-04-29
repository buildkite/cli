package resolver

import (
	"context"
	"errors"

	"github.com/buildkite/cli/v3/internal/pipeline"
)

// PipelineResolverFn is a function for the purpose of finding a pipeline. It returns an error if an irrecoverable
// scenario happens and should halt execution. Otherwise if the resolver does not find a pipeline, it should return
// (nil, nil) to indicate this. ie. no error occurred, but no pipeline was found either.
type PipelineResolverFn func(context.Context) (*pipeline.Pipeline, error)

type AggregateResolver []PipelineResolverFn

// Resolve is a PipelineResolverFn that wraps up a list of resolvers to loop through to try find a pipeline. The first
// pipeline that is found will be returned, if none are found if won't return an error to match the expectation of a
// PipelineResolveFn
//
// This is safe to call multiple times. The same result will be returned.
func (pr AggregateResolver) Resolve(ctx context.Context) (*pipeline.Pipeline, error) {
	for _, resolve := range pr {
		p, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}

	return nil, nil
}

// NewAggregateResolver creates an AggregregateResolver from a list of PipelineResolverFn, appending a final resolver
// for capturing the case that no resolvers find a pipeline
func NewAggregateResolver(resolvers ...PipelineResolverFn) AggregateResolver {
	// add a final error resolver to the chain in case no other resolvers find a pipeline
	return append(resolvers, errorResolver)
}

func errorResolver(context.Context) (*pipeline.Pipeline, error) {
	return nil, errors.New("failed to resolve a pipeline")
}
