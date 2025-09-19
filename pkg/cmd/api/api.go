package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Khan/genqlient/graphql"
	"github.com/MakeNowJust/heredoc"
	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

var (
	method    string
	headers   []string
	data      string
	analytics bool
	queryFile string
)

func NewCmdAPI(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "api <endpoint>",
		Short: "Interact with the Buildkite API",
		Long:  "Interact with either the REST or GraphQL Buildkite APIs",
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
      # To get a build
      $ bk api /pipelines/example-pipeline/builds/420

      # To create a pipeline
      $ bk api --method POST /pipelines --data '
      {
        "name": "My Cool Pipeline",
        "repository": "git@github.com:acme-inc/my-pipeline.git",
        "configuration": "steps:\n - command: env"
      }
      '

      # To update a cluster
      $ bk api --method PUT /clusters/CLUSTER_UUID --data '
      {
        "name": "My Updated Cluster",
      }
      '

      # To get all test suites
      $ bk api --analytics /suites

      # Run GraphQL query from file
      $ bk api --file get_build.graphql
      `),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		RunE: func(cmd *cobra.Command, args []string) error {
			if data != "" && !cmd.Flags().Changed("method") {
				method = "POST"
			}
			return apiCaller(cmd, args, f)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", "GET", "HTTP method to use")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", []string{}, "Headers to include in the request")
	cmd.Flags().StringVarP(&data, "data", "d", "", "Data to send in the request body")
	cmd.Flags().BoolVar(&analytics, "analytics", false, "Use the Test Analytics endpoint")
	cmd.Flags().StringVarP(&queryFile, "file", "f", "", "File containing GraphQL query")

	return &cmd
}

func apiCaller(cmd *cobra.Command, args []string, f *factory.Factory) error {
	// Handle GraphQL file queries
	if queryFile != "" {
		return handleGraphQLQuery(cmd, f)
	}

	var endpoint string
	var endpointPrefix string

	if len(args) > 1 {
		return fmt.Errorf("incorrect number of arguments. expected 1, got %d", len(args))
	}

	if len(args) == 0 {
		endpoint = "/"
	} else {
		endpoint = args[0]
	}

	if analytics {
		endpointPrefix = fmt.Sprintf("v2/analytics/organizations/%s", f.Config.OrganizationSlug())
	} else {
		endpointPrefix = fmt.Sprintf("v2/organizations/%s", f.Config.OrganizationSlug())
	}

	fullEndpoint := endpointPrefix + endpoint

	// Create an HTTP client with appropriate configuration
	client := httpClient.NewClient(
		f.Config.APIToken(),
		httpClient.WithBaseURL(f.RestAPIClient.BaseURL.String()),
	)

	// Process custom headers
	customHeaders := make(map[string]string)
	for _, header := range headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			customHeaders[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	var requestData interface{}
	if data != "" {
		// Try to parse as JSON first
		if err := json.Unmarshal([]byte(data), &requestData); err != nil {
			// If not JSON, use raw string
			requestData = data
		}
	}

	var response interface{}
	var err error

	switch method {
	case "GET":
		err = client.Get(cmd.Context(), fullEndpoint, &response)
	case "POST":
		err = client.Post(cmd.Context(), fullEndpoint, requestData, &response)
	case "PUT":
		err = client.Put(cmd.Context(), fullEndpoint, requestData, &response)
	default:
		// For other methods, use the Do method directly
		err = client.Do(cmd.Context(), method, fullEndpoint, requestData, &response)
	}

	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}

	// Format and print the response
	var prettyJSON bytes.Buffer
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	err = json.Indent(&prettyJSON, responseBytes, "", "  ")
	if err != nil {
		return fmt.Errorf("error formatting JSON response: %w", err)
	}

	fmt.Println(prettyJSON.String())

	return nil
}

func handleGraphQLQuery(cmd *cobra.Command, f *factory.Factory) error {
	// Read the GraphQL query from file
	queryBytes, err := os.ReadFile(queryFile)
	if err != nil {
		return fmt.Errorf("error reading GraphQL query file %s: %w", queryFile, err)
	}

	// Validate and parse GraphQL query
	query := strings.TrimSpace(string(queryBytes))
	if query == "" {
		return fmt.Errorf("GraphQL query file %s is empty", queryFile)
	}

	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return fmt.Errorf("invalid GraphQL query: %w", err)
	}

	// Validate that we have at least one operation
	if len(doc.Operations) == 0 {
		return fmt.Errorf("GraphQL query must contain at least one operation (query, mutation, or subscription)")
	}

	// Extract and validate operation name (Buildkite GraphQL API requires named operations)
	opName := doc.Operations[0].Name
	if opName == "" {
		return fmt.Errorf("GraphQL operation must have a name when using file input. Please add a name after the operation type, e.g., 'query MyQuery { ... }'")
	}

	// Create GraphQL request using the existing client infrastructure
	req := &graphql.Request{
		OpName: opName,
		Query:  query,
	}

	// Use a generic response type for raw queries
	resp := &graphql.Response{Data: new(interface{})}

	// Use the existing GraphQL client
	if err = f.GraphQLClient.MakeRequest(cmd.Context(), req, resp); err != nil {
		return fmt.Errorf("error making GraphQL request: %w", err)
	}

	// Format and print the response
	responseBytes, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	var prettyJSON bytes.Buffer
	if err = json.Indent(&prettyJSON, responseBytes, "", "  "); err != nil {
		return fmt.Errorf("error formatting JSON response: %w", err)
	}

	fmt.Println(prettyJSON.String())
	return nil
}
