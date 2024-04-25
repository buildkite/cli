package pipeline

import (
	"strings"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
)

func ResolveFromPath(path string, org string, client *buildkite.Client) ([]string, error) {
	repos, err := getRepoURLs(path)
	if err != nil {
		return nil, err
	}
	return filterPipelines(repos, org, client)
}

func filterPipelines(repoURLs []string, org string, client *buildkite.Client) ([]string, error) {
	var currentPipelines []string
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
					currentPipelines = append(currentPipelines, *p.Slug)

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

func getRepoURLs(path string) ([]string, error) {
	searchPath := "." // default to current directory
	if len(path) > 0 {
		searchPath = path
	}
	r, err := git.PlainOpenWithOptions(searchPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}

	c, err := r.Config()
	if err != nil {
		return nil, err
	}
	return c.Remotes["origin"].URLs, nil
}
