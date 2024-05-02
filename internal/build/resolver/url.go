package resolver

import (
	"context"
	"fmt"
	"regexp"

	"github.com/buildkite/cli/v3/internal/build"
)

func ResolveFromURL(args []string) BuildResolverFn {
	return func(context.Context) (*build.Build, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("Incorrect number of arguments, expected 1, got %d", len(args))
		}
		resolvedURL := splitBuildURL(args[0])

		org, name, buildNumber := resolvedURL.Organization, resolvedURL.Pipeline, resolvedURL.BuildNumber

		return &build.Build{Pipeline: name, Organization: org, BuildNumber: buildNumber}, nil
	}
}

func splitBuildURL(url string) build.Build {
	re := regexp.MustCompile(`https://buildkite.com/([^/]+)/([^/]+)/builds/(\d+)$`)
	matches := re.FindStringSubmatch(url)

	return build.Build{
		Organization: matches[1],
		Pipeline:     matches[2],
		BuildNumber:  matches[3],
	}
}
