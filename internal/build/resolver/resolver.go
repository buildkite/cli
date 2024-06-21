package resolver

import (
	"context"
	"errors"

	"github.com/buildkite/cli/v3/internal/build"
)

// BuildResolverFn is a function for finding a build. It returns an error if an irrecoverable scenario happens and
// should halt execution. Otherwise, if the resolver does not find a build, it should return (nil, nil) to indicate
// this. ie. no error occurred, but no build was found either
type BuildResolverFn func(context.Context) (*build.Build, error)

type AggregateResolver []BuildResolverFn

// Resolve is a BuildResolverFn that wraps up a list of resolvers to loop through and try find a build. The first build
// to be found will be returned. If none are found, it won't return an error to match the expectation of a
// BuildResolverFn
//
// This is safe to call multiple times, the same result will be returned
func (ar AggregateResolver) Resolve(ctx context.Context) (*build.Build, error) {
	for _, resolve := range ar {
		b, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		if b != nil {
			return b, nil
		}
	}

	return nil, nil
}

func (ar AggregateResolver) WithResolverWhen(condition bool, resovler BuildResolverFn) AggregateResolver {
	if condition {
		return append(ar, resovler)
	}
	return ar
}

// NewAggregateResolver creates an AggregateResolver from a list of BuildResolverFn, appending a final resolver for
// capturing the case that no build is found by any resolver
func NewAggregateResolver(resolvers ...BuildResolverFn) AggregateResolver {
	return append(resolvers, errorResolver)
}

func errorResolver(context.Context) (*build.Build, error) {
	return nil, errors.New("Failed to find a build.")
}
