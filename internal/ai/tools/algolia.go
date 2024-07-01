package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/internal/algolia"
	"github.com/gptscript-ai/gptscript/pkg/types"
)

// AlgoliaTool is a gptscript tool that searches algolia for a given term and returns URLs to documentation pages
func AlgoliaTool() types.Tool {
	return types.Tool{
		ToolDef: types.ToolDef{
			Parameters: types.Parameters{
				Name:        "algolia",
				Description: "Query the Buildkite documentation at https://buildkite.com for relevant URLs",
				Arguments:   types.ObjectSchema("query", "The query to search for"),
			},
			BuiltinFunc: func(ctx context.Context, env []string, input string, progress chan<- string) (string, error) {
				results, err := algolia.Search(input)
				if err != nil {
					return "", nil
				}
				if len(results) <= 0 {
					return "", fmt.Errorf("could not find any algolia hits for search: %s", input)
				}

				return strings.Join(results, "\n"), nil
			},
		},
	}
}
