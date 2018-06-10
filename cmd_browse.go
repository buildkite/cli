package cli

import (
	"fmt"
	"path/filepath"

	"github.com/buildkite/cli/git"
	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"
	"github.com/manifoldco/promptui"
	"github.com/skratchdot/open-golang/open"
)

type BrowseCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Dir    string
	Branch string
}

func BrowseCommand(ctx BrowseCommandContext) error {
	dir, err := filepath.Abs(ctx.Dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	branch := ctx.Branch
	if branch == "" {
		var gitErr error
		branch, gitErr = git.Branch(dir)
		if gitErr != nil {
			return NewExitError(err, 1)
		}
	}

	gitRemote, err := git.Remote(dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelines, err := findPipelinesByGitRemote(bk, gitRemote)
	if err != nil {
		return NewExitError(err, 1)
	}

	prompt := promptui.Select{
		Label: "Select pipeline",
		Items: pipelines,
	}

	_, pipelineURL, err := prompt.Run()
	if err != nil {
		return NewExitError(err, 1)
	}

	if pipelineURL != "" {
		pipelineURL += "/builds?branch=" + branch
	}

	if err := open.Run(pipelineURL); err != nil {
		return NewExitError(err, 1)
	}

	return nil
}

func findPipelinesByGitRemote(client *graphql.Client, gitRemote string) ([]string, error) {
	resp, err := client.Do(`
		query {
			viewer {
				organizations {
					edges {
						node {
							pipelines(first: 500) {
								edges {
									node {
										url
										repository {
											url
							 			}
									}
								}
							}
						}
					}
				}
			}
		 }
	`, nil)
	if err != nil {
		return nil, err
	}

	var parsedResp struct {
		Data struct {
			Viewer struct {
				Organizations struct {
					Edges []struct {
						Node struct {
							Pipelines struct {
								Edges []struct {
									Node struct {
										URL        string `json:"url"`
										Repository struct {
											URL string `json:"url"`
										} `json:"repository"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"pipelines"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"organizations"`
			} `json:"viewer"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return nil, fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	org, repo, err := github.ParseGithubRemote(gitRemote)
	if err != nil {
		return nil, err
	}

	var pipelines []string

	for _, orgEdge := range parsedResp.Data.Viewer.Organizations.Edges {
		for _, pipelineEdge := range orgEdge.Node.Pipelines.Edges {
			pipelineOrg, pipelineRepo, err := github.ParseGithubRemote(pipelineEdge.Node.Repository.URL)
			if err != nil {
				debugf("Error parsing remote: %v", err)
				continue
			}

			if pipelineOrg == org && pipelineRepo == repo {
				pipelines = append(pipelines, pipelineEdge.Node.URL)
			}
		}
	}
	return pipelines, nil
}
