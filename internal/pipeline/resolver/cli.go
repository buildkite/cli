package resolver

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
)

func ResolveFromPositionalArgument(args []string, index int, conf *config.Config) PipelineResolverFn {
	return func(context.Context) (*pipeline.Pipeline, error) {
		// if args does not have values, skip this resolver
		if len(args) < 1 {
			return nil, nil
		}
		// if the index is out of bounds
		if (len(args) - 1) < index {
			return nil, nil
		}

		org, name := parsePipelineArg(args[index], conf)

		// if we get here, we should be able to parse the value and return an error if not
		// this is because a user has explicitly given an input value for us to use - we shoulnt ignore it on error
		if org == "" || name == "" {
			return nil, fmt.Errorf("unable to parse the input pipeline argument: \"%s\"", args[index])
		}

		return &pipeline.Pipeline{Name: name, Org: org}, nil
	}
}

// parsePipelineArg resolve an input string in varying formats to an organization and pipeline pair
// some example input formats are:
// - a web URL: https://buildkite.com/<org>/<pipeline slug>/builds/...
// - a slug: <org>/<pipeline slug>
// - a pipeline slug by itself
func parsePipelineArg(arg string, conf *config.Config) (org, pipeline string) {
	pipelineIsURL := strings.Contains(arg, ":")
	pipelineIsSlug := !pipelineIsURL && strings.Contains(arg, "/")

	if pipelineIsURL {
		url, err := url.Parse(arg)
		if err != nil {
			return "", ""
		}
		// eg: url.Path = /buildkite/buildkite-cli
		part := strings.Split(url.Path, "/")
		if len(part) < 3 {
			return "", ""
		}
		org, pipeline = part[1], part[2]
	} else if pipelineIsSlug {
		part := strings.Split(arg, "/")
		if len(part) < 2 {
			return "", ""
		}
		org, pipeline = part[0], part[1]
	} else {
		org = conf.OrganizationSlug()
		pipeline = arg
	}
	return org, pipeline
}
