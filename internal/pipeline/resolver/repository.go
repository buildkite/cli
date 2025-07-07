package resolver

import (
	"context"
	"strings"

	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
	git "github.com/go-git/go-git/v5"
)

// ResolveFromRepository finds pipelines based on the current repository.
//
// It queries the API for all pipelines in the organization that match the repository's URL.
// It delegates picking one from the list of matches to the `picker`.
func ResolveFromRepository(f *factory.Factory, picker PipelinePicker) PipelineResolverFn {
	return func(ctx context.Context) (*pipeline.Pipeline, error) {
		var err error
		var pipelines []pipeline.Pipeline
		spinErr := spinner.New().
			Title("Resolving pipeline").
			Action(func() {
				pipelines, err = resolveFromRepository(ctx, f)
			}).
			Run()
		if spinErr != nil {
			return nil, spinErr
		}
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

func resolveFromRepository(ctx context.Context, f *factory.Factory) ([]pipeline.Pipeline, error) {
	repos, err := getRepoURLs(f.GitRepository)
	if err != nil {
		return nil, err
	}
	return filterPipelines(ctx, repos, f.Config.OrganizationSlug(), f.RestAPIClient)
}

func filterPipelines(ctx context.Context, repoURLs []string, org string, client *buildkite.Client) ([]pipeline.Pipeline, error) {
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

		pipelines, resp, err := client.Pipelines.List(ctx, org, &opts)
		if err != nil {
			return nil, err
		}
		for _, p := range pipelines {
			for _, u := range repoURLs {
				gitUrl := u[strings.LastIndex(u, "/")+1:]
				if strings.Contains(p.Repository, gitUrl) {
					currentPipelines = append(currentPipelines, pipeline.Pipeline{Name: p.Slug, Org: org})
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
		return nil, nil // could not resolve to any repository, proceed to another resolver
	}

	c, err := r.Config()
	if err != nil {
		return nil, err
	}

	if _, ok := c.Remotes["origin"]; !ok {
		return nil, nil // repo's "origin" remote does not exist, proceed to another resolver
	}
	return c.Remotes["origin"].URLs, nil
}
