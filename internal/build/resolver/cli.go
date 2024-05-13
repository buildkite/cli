package resolver

import (
	"context"
	"strconv"
	"strings"

	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/internal/config"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
)

func ResolveFromPositionalArgument(args []string, index int, pipeline pipelineResolver.PipelineResolverFn, conf *config.Config) BuildResolverFn {
	return func(ctx context.Context) (*build.Build, error) {
		// if args does not have values, skip this resolver
		if len(args) < 1 {
			return nil, nil
		}
		// if the index is out of bounds
		if (len(args) - 1) < index {
			return nil, nil
		}

		build := parseBuildArg(ctx, args[index], pipeline)

		return build, nil
	}
}

func parseBuildArg(ctx context.Context, arg string, pipeline pipelineResolver.PipelineResolverFn) *build.Build {
	buildIsURL := strings.Contains(arg, ":")
	buildIsSlug := !buildIsURL && strings.Contains(arg, "/")

	if buildIsURL {
		return splitBuildURL(arg)
	} else if buildIsSlug {
		part := strings.Split(arg, "/")
		if len(part) < 3 {
			return nil
		}
		num, err := strconv.Atoi(part[2])
		if err != nil {
			return nil
		}
		return &build.Build{
			Organization: part[0],
			Pipeline:     part[1],
			BuildNumber:  num,
		}
	}

	num, err := strconv.Atoi(arg)
	if err != nil {
		return nil
	}
	p, err := pipeline(ctx)
	if err != nil || p == nil {
		return nil
	}
	return &build.Build{
		Organization: p.Org,
		Pipeline:     p.Name,
		BuildNumber:  num,
	}
}
