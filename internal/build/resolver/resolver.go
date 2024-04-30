package resolver

import (
	"context"
	"errors"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

var resolved bool
var resolvedBuild *build.Build
var resolvedError error

// BuildResolverFn is a function for finding a build. It returns an error if an irrecoverable scenario happens and
// should halt execution. Otherwise, if the resolver does not find a build, it should return (nil, nil) to indicate
// this. ie. no error occurred, but no build was found either
//
// Some build resolvers may need to limit scope to a pipeline so that is provided to the resolver at runtime
type BuildResolverFn func(context.Context, pipeline.Pipeline) (*build.Build, error)

type AggregateResolver []BuildResolverFn

// Resolve is a BuildResolverFn that wraps up a list of resolvers to loop through and try find a build. The first build
// to be found will be returned. If none are found, it won't return an error to match the expectation of a
// BuildResolverFn
//
// This is safe to call multiple times, the same result will be returned
func (ar AggregateResolver) Resolve(ctx context.Context, p pipeline.Pipeline) (*build.Build, error) {
	// short-circuit if this has been called before
	if resolved {
		return resolvedBuild, resolvedError
	}

	resolved = true

	for _, resolve := range ar {
		b, err := resolve(ctx, p)
		if err != nil {
			resolvedError = err
			return nil, err
		}
		if b != nil {
			resolvedBuild = b
			return b, nil
		}
	}

	return nil, nil
}

// NewAggregateResolver creates an AggregateResolver from a list of BuildResolverFn, appending a final resolver for
// capturing the case that no build is found by any resolver
func NewAggregateResolver(resolvers ...BuildResolverFn) AggregateResolver {
	return append(resolvers, errorResolver)
}

func errorResolver(context.Context, pipeline.Pipeline) (*build.Build, error) {
	return nil, errors.New("Failed to find a build.")
}
