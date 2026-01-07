package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

type ApiCmd struct {
	Endpoint  string   `arg:"" optional:"" help:"API endpoint to call"`
	Method    string   `help:"HTTP method to use" short:"X"`
	Headers   []string `help:"Headers to include in the request" short:"H"`
	Data      string   `help:"Data to send in the request body" short:"d"`
	Analytics bool     `help:"Use the Test Analytics endpoint"`
	File      string   `help:"File containing GraphQL query" short:"f"`
	Verbose   bool     `help:"Enable verbose output (currently only provides information about rate limit exceeded retries)"`
}

func (c *ApiCmd) Help() string {
	return `
Interact with either the REST or GraphQL Buildkite APIs.

Examples:
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
`
}

// buildFullEndpoint constructs the full API endpoint path with organization prefix
func buildFullEndpoint(endpoint, orgSlug string, isAnalytics bool) string {
	// Default to root if empty
	if endpoint == "" {
		endpoint = "/"
	}

	// Ensure endpoint starts with a leading slash
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	var endpointPrefix string
	if isAnalytics {
		endpointPrefix = fmt.Sprintf("v2/analytics/organizations/%s", orgSlug)
	} else {
		endpointPrefix = fmt.Sprintf("v2/organizations/%s", orgSlug)
	}

	return endpointPrefix + endpoint
}

func (c *ApiCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	// Determine HTTP method: default to GET, but use POST if data is provided and method not explicitly set
	method := c.Method
	if method == "" {
		if c.Data != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	// Handle GraphQL file queries
	if c.File != "" {
		return c.handleGraphQLQuery(context.Background(), f)
	}

	fullEndpoint := buildFullEndpoint(c.Endpoint, f.Config.OrganizationSlug(), c.Analytics)

	// Create an HTTP client with appropriate configuration
	client := httpClient.NewClient(
		f.Config.APIToken(),
		httpClient.WithBaseURL(f.RestAPIClient.BaseURL.String()),
		httpClient.WithMaxRetries(3),
		httpClient.WithMaxRetryDelay(60*time.Second),
		httpClient.WithOnRetry(func(attempt int, delay time.Duration) {
			if c.Verbose {
				fmt.Fprintf(os.Stderr, "WARNING: Rate limit exceeded, retrying in %v @ %q (attempt %d)\n", delay, time.Now().Add(delay).Format(time.RFC3339), attempt)
			}
		}),
	)

	// Process custom headers
	customHeaders := make(map[string]string)
	for _, header := range c.Headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			customHeaders[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	var requestData any
	if c.Data != "" {
		// Try to parse as JSON first
		if err := json.Unmarshal([]byte(c.Data), &requestData); err != nil {
			// If not JSON, use raw string
			requestData = c.Data
		}
	}

	var response any

	switch method {
	case "GET":
		err = client.Get(context.Background(), fullEndpoint, &response)
	case "POST":
		err = client.Post(context.Background(), fullEndpoint, requestData, &response)
	case "PUT":
		err = client.Put(context.Background(), fullEndpoint, requestData, &response)
	default:
		// For other methods, use the Do method directly
		err = client.Do(context.Background(), method, fullEndpoint, requestData, &response)
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

func (c *ApiCmd) handleGraphQLQuery(ctx context.Context, f *factory.Factory) error {
	// Read the GraphQL query from file
	queryBytes, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("error reading GraphQL query file %s: %w", c.File, err)
	}

	// Validate and parse GraphQL query
	query := strings.TrimSpace(string(queryBytes))
	if query == "" {
		return fmt.Errorf("GraphQL query file %s is empty", c.File)
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
	if err = f.GraphQLClient.MakeRequest(ctx, req, resp); err != nil {
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
