package cli

import (
	"fmt"

	"github.com/buildkite/cli/graphql"
)

type pipeline struct {
	ID            string
	Org           string
	Slug          string
	URL           string
	RepositoryURL string
}

func listPipelines(client *graphql.Client) ([]pipeline, error) {
	resp, err := client.Do(`
		query {
			viewer {
			  organizations {
				edges {
				  node {
					slug
					pipelines(first:500) {
					  edges {
						node {
							id
							slug
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
							Slug      string `json:"slug"`
							Pipelines struct {
								Edges []struct {
									Node struct {
										ID         string `json:"id"`
										Slug       string `json:"slug"`
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

	var pipelines []pipeline

	for _, orgEdge := range parsedResp.Data.Viewer.Organizations.Edges {
		for _, pipelineEdge := range orgEdge.Node.Pipelines.Edges {
			pipelines = append(pipelines, pipeline{
				ID:            pipelineEdge.Node.ID,
				URL:           pipelineEdge.Node.URL,
				Org:           orgEdge.Node.Slug,
				Slug:          pipelineEdge.Node.Slug,
				RepositoryURL: pipelineEdge.Node.Repository.URL,
			})
		}
	}
	return pipelines, nil
}
