package resolver

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/buildkite/cli/v3/internal/build"
)

func ResolveFromURL(args []string) BuildResolverFn {
	return func(context.Context) (*build.Build, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("Incorrect number of arguments, expected 1, got %d", len(args))
		}
		resolvedBuild := splitBuildURL(args[0])

		if resolvedBuild == nil {
			return nil, fmt.Errorf("Unable to resolve build from URL: %s", args[0])
		}

		return resolvedBuild, nil
	}
}

func splitBuildURL(url string) *build.Build {
	re := regexp.MustCompile(`https://buildkite.com/([^/]+)/([^/]+)/builds/(\d+)$`)
	matches := re.FindStringSubmatch(url)
	if matches == nil || len(matches) != 4 {
		return nil
	}

	num, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil
	}

	return &build.Build{
		Organization: matches[1],
		Pipeline:     matches[2],
		BuildNumber:  num,
	}
}
