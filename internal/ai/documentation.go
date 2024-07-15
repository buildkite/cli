package ai

import (
	"encoding/json"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type DocumentationTool struct {
	// a function that accepts a search query string and returns a list of matching URLs
	Search func(string) ([]string, error)
}

func (t *DocumentationTool) ToolDefinition() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "buildkite_documentation",
			Description: "Search the buildkite documentation for a list of relevant URLs",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"query": {
						Type:        jsonschema.String,
						Description: "The search query",
					},
				},
				Required: []string{"query"},
			},
		},
	}
}

// Execute will perform this tools function.
//
// This tool will search algolia for URLs to documentation pages and return a list
func (t *DocumentationTool) Execute(args string) (any, error) {
	var query struct {
		Query string `json:"query"`
	}
	err := json.Unmarshal([]byte(args), &query)
	if err != nil {
		return nil, err
	}

	urls, err := t.Search(query.Query)
	if err != nil {
		return nil, err
	}
	return urls, nil
}
