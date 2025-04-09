package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
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
