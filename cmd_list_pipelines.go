package cli

import (
	"fmt"

	"github.com/buildkite/cli/graphql"
	"github.com/sahilm/fuzzy"
)

type ListPipelinesCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Fuzzy   string
	Limit   int
	ShowURL bool
}

func ListPipelinesCommand(ctx ListPipelinesCommandContext) error {
	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelines, err := listPipelines(bk)
	if err != nil {
		return NewExitError(err, 1)
	}

	var counter int

	if ctx.Fuzzy != "" {
		const bold = "\033[1m%s\033[0m"
		matches := fuzzy.Find(ctx.Fuzzy, pipelines)

		for _, match := range matches {
			counter++
			if ctx.Limit > 0 && counter > ctx.Limit {
				break
			}

			if ctx.ShowURL {
				ctx.Printf(`https://buildkite.com/`)
			}
			for i := 0; i < len(match.Str); i++ {
				if contains(i, match.MatchedIndexes) {
					fmt.Print(fmt.Sprintf(bold, string(match.Str[i])))
				} else {
					fmt.Print(string(match.Str[i]))
				}

			}
			fmt.Println()
		}

		return nil
	}

	for _, p := range pipelines {
		counter++
		if ctx.Limit > 0 && counter > ctx.Limit {
			break
		}
		if ctx.ShowURL {
			ctx.Printf(`https://buildkite.com/`)
		}
		ctx.Println(p)
	}

	return nil
}

func contains(needle int, haystack []int) bool {
	for _, i := range haystack {
		if needle == i {
			return true
		}
	}
	return false
}

func listPipelines(client *graphql.Client) ([]string, error) {
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
							slug
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
										Slug string `json:"slug"`
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

	var pipelines []string

	for _, org := range parsedResp.Data.Viewer.Organizations.Edges {
		for _, pipeline := range org.Node.Pipelines.Edges {
			pipelines = append(pipelines, fmt.Sprintf("%s/%s",
				org.Node.Slug, pipeline.Node.Slug))
		}
	}
	return pipelines, nil
}
