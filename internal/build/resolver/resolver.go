package resolver

import "github.com/buildkite/cli/v3/internal/build"

type BuildResolverFn func() (*build.Build, error)
