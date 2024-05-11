package resolver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
)

// ResolveFromRepository finds pipelines based on the current repository.
//
// It queries the API for all pipelines in the organization that match the repository's URL.
// It delegates picking one from the list of matches to the `picker`.
func ResolveFromRepository(f *factory.Factory, picker PipelinePicker) PipelineResolverFn {
	return func(ctx context.Context) (*pipeline.Pipeline, error) {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Suffix = " Resolving pipeline"
		s.Start()
		pipelines, err := resolveFromRepository(f)
		s.Stop()
		if err != nil {
			return nil, err
		}
		if len(pipelines) == 0 {
			return nil, nil
		}
		pipeline := picker(pipelines)
		if pipeline == nil {
			return nil, nil
		}

		return pipeline, nil
	}
}

func resolveFromRepository(f *factory.Factory) ([]pipeline.Pipeline, error) {
	repos, err := getRepoURLs(f.GitRepository)
	if err != nil {
		return nil, err
	}
	return filterPipelines(repos, f.Config.OrganizationSlug(), f.RestAPIClient)
}

func filterPipelines(repoURLs []string, org string, client *buildkite.Client) ([]pipeline.Pipeline, error) {
	var currentPipelines []pipeline.Pipeline
	page := 1
	per_page := 30
	for more_pipelines := true; more_pipelines; {
		opts := buildkite.PipelineListOptions{
			ListOptions: buildkite.ListOptions{
				Page:    page,
				PerPage: per_page,
			},
		}

		pipelines, resp, err := client.Pipelines.List(org, &opts)
		if err != nil {
			return nil, err
		}
		for _, p := range pipelines {
			for _, u := range repoURLs {
				gitUrl := u[strings.LastIndex(u, "/")+1:]
				if strings.Contains(*p.Repository, gitUrl) {
					currentPipelines = append(currentPipelines, pipeline.Pipeline{Name: *p.Slug, Org: org})
				}
			}
		}
		if resp.NextPage == 0 {
			more_pipelines = false
		} else {
			page = resp.NextPage
		}
	}
	return currentPipelines, nil
}

func getRepoURLs(r *git.Repository) ([]string, error) {
	if r == nil {
		return nil, fmt.Errorf("could not determine current repository")
	}

	c, err := r.Config()
	if err != nil {
		return nil, err
	}
	return c.Remotes["origin"].URLs, nil
}
