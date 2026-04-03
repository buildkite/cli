package resolver

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"

	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

type repositoryQuery struct {
	identity string
	search   string
}

// ResolveFromRepository finds pipelines based on the current repository.
//
// It queries the API for all pipelines in the organization that match the repository's URL.
// It delegates picking one from the list of matches to the `picker`.
func ResolveFromRepository(f *factory.Factory, picker PipelinePicker) PipelineResolverFn {
	return resolveFromRepositoryWithOrg(f, picker, f.Config.OrganizationSlug())
}

// ResolveFromRepositoryInOrg finds pipelines in a specific organization based
// on the current repository.
func ResolveFromRepositoryInOrg(f *factory.Factory, picker PipelinePicker, org string) PipelineResolverFn {
	return resolveFromRepositoryWithOrg(f, picker, org)
}

func resolveFromRepositoryWithOrg(f *factory.Factory, picker PipelinePicker, org string) PipelineResolverFn {
	return func(ctx context.Context) (*pipeline.Pipeline, error) {
		var err error
		var pipelines []pipeline.Pipeline
		spinErr := bkIO.SpinWhile(f, "Resolving pipeline", func() {
			pipelines, err = resolveFromRepository(ctx, f, org)
		})
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

func resolveFromRepository(ctx context.Context, f *factory.Factory, org string) ([]pipeline.Pipeline, error) {
	repos, err := getRepoURLs(f.GitRepository)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		repos, err = getRepoURLsFromGit(ctx)
		if err != nil {
			return nil, err
		}
	}
	return filterPipelines(ctx, repos, org, f.RestAPIClient)
}

func filterPipelines(ctx context.Context, repoURLs []string, org string, client *buildkite.Client) ([]pipeline.Pipeline, error) {
	queries := repositoryQueries(repoURLs)
	if len(queries) == 0 {
		return nil, nil
	}

	var currentPipelines []pipeline.Pipeline
	seen := make(map[string]struct{})
	perPage := 30
	for _, repoURL := range uniqueRepositorySearchTerms(queries) {
		page := 1
		for more_pipelines := true; more_pipelines; {
			opts := buildkite.PipelineListOptions{
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
				Repository: repoURL,
			}

			pipelines, resp, err := client.Pipelines.List(ctx, org, &opts)
			if err != nil {
				return nil, err
			}
			for _, p := range pipelines {
				if !matchesRepositoryQuery(p.Repository, queries) {
					continue
				}
				if _, ok := seen[p.Slug]; ok {
					continue
				}
				seen[p.Slug] = struct{}{}
				currentPipelines = append(currentPipelines, pipeline.Pipeline{Name: p.Slug, Org: org})
			}
			if resp.NextPage == 0 {
				more_pipelines = false
			} else {
				page = resp.NextPage
			}
		}
	}
	return currentPipelines, nil
}

func repositoryQueries(repoURLs []string) []repositoryQuery {
	queries := make([]repositoryQuery, 0, len(repoURLs))
	seen := make(map[string]struct{}, len(repoURLs))
	for _, repoURL := range repoURLs {
		identity, search := normalizeRepositoryURL(repoURL)
		if identity == "" || search == "" {
			continue
		}
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		queries = append(queries, repositoryQuery{identity: identity, search: search})
	}
	return queries
}

func uniqueRepositorySearchTerms(queries []repositoryQuery) []string {
	terms := make([]string, 0, len(queries))
	seen := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		if _, ok := seen[query.search]; ok {
			continue
		}
		seen[query.search] = struct{}{}
		terms = append(terms, query.search)
	}
	return terms
}

func matchesRepositoryQuery(repository string, queries []repositoryQuery) bool {
	identity, _ := normalizeRepositoryURL(repository)
	if identity == "" {
		return false
	}

	for _, query := range queries {
		if identity == query.identity {
			return true
		}
	}

	return false
}

func normalizeRepositoryURL(repository string) (identity string, search string) {
	endpoint, err := transport.NewEndpoint(repository)
	if err != nil {
		return "", ""
	}

	search = normalizeRepositoryPath(endpoint.Path)
	if search == "" {
		return "", ""
	}

	identity = search
	if host := normalizeRepositoryHost(endpoint.Protocol, endpoint.Host, endpoint.Port); host != "" {
		identity = host + "/" + search
	}

	return identity, search
}

func normalizeRepositoryPath(repoPath string) string {
	repoPath = strings.ToLower(strings.TrimSpace(repoPath))
	repoPath = strings.Trim(repoPath, "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	return strings.Trim(repoPath, "/")
}

func normalizeRepositoryHost(protocol, host string, port int) string {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	if host == "" {
		return ""
	}
	if port == 0 || isDefaultRepositoryPort(protocol, port) {
		return host
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

func isDefaultRepositoryPort(protocol string, port int) bool {
	switch strings.ToLower(protocol) {
	case "http":
		return port == 80
	case "https":
		return port == 443
	case "ssh":
		return port == 22
	case "git":
		return port == 9418
	default:
		return false
	}
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

func getRepoURLsFromGit(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "--all", "origin")
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		var execErr *exec.Error
		if errors.As(err, &exitErr) || errors.As(err, &execErr) {
			return nil, nil
		}
		return nil, err
	}

	var urls []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		url := strings.TrimSpace(line)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
	}

	return urls, nil
}
