package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/go-git/go-git/v5"
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
				pipelines, err = resolveFromRepository(f)
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

func resolveFromRepository(f *factory.Factory) ([]pipeline.Pipeline, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.New("Could not resolve current working directory")
	}
	resolvedPipelines := make([]pipeline.Pipeline, 0)
	repos, err := getRepoURLs(f.GitRepository)
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		foundPipelines, err := findPipelinesFromRepo(repo, f.Config.OrganizationSlug(), f)
		if err != nil {
			continue
		}

		resolvedPipelines = append(resolvedPipelines, foundPipelines...)
	}

	cwdPipelines, err := findPipelinesFromCwd(filepath.Base(cwd), f.Config.OrganizationSlug(), f)
	if err != nil {
		return resolvedPipelines, nil
	}

	resolvedPipelines = append(resolvedPipelines, cwdPipelines...)

	return resolvedPipelines, nil
}

func findPipelinesFromCwd(cwd string, org string, f *factory.Factory) ([]pipeline.Pipeline, error) {
	resolvedPipelines := make([]pipeline.Pipeline, 0)
	res, err := graphql.FindPipelineFromCwd(context.Background(), f.GraphQLClient, org, &cwd)
	if err != nil {
		return nil, err
	}

	for _, p := range res.Organization.Pipelines.Edges {
		resolvedPipelines = append(resolvedPipelines, pipeline.Pipeline{Name: p.Node.GetName(), Org: p.Node.Organization.GetName()})
	}

	return resolvedPipelines, nil
}

func findPipelinesFromRepo(repo string, org string, f *factory.Factory) ([]pipeline.Pipeline, error) {
	resolvedPipelines := make([]pipeline.Pipeline, 0)
	res, err := graphql.FindPipelineFromGitRepoUrl(context.Background(), f.GraphQLClient, org, repo)
	if err != nil {
		return nil, err
	}

	for _, p := range res.Organization.Pipelines.Edges {
		resolvedPipelines = append(resolvedPipelines, pipeline.Pipeline{Name: p.Node.GetName(), Org: p.Node.Organization.GetName()})
	}
	return resolvedPipelines, nil
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
