package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/factory"
)

// API command
type APICmd struct {
	Method    string     `help:"HTTP method (default: GET or POST if --data is set)"`
	Header    HeaderFlag `help:"HTTP header(s) (KEY=VAL or KEY: VAL)" name:"header"`
	Data      string     `short:"d" help:"Request body data"`
	Analytics bool       `help:"Include analytics data"`
	File      string     `help:"File to upload"`
	Path      string     `arg:"" help:"API path"`
}

func (a *APICmd) Help() string {
	return `EXAMPLES:
  # Get organization info
  bk api ""

  # List builds
  bk api builds

  # List builds with custom headers
  bk api builds --header "Content-Type: application/json"

  # Create a build with POST data
  bk api pipelines/my-pipeline/builds --data '{"commit":"HEAD","branch":"main"}'`
}

func (a *APICmd) Run(ctx context.Context, f *factory.Factory) error {
	// Validate configuration
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Set default method based on data presence
	method := a.Method
	if a.Data != "" && method == "" {
		method = "POST"
	}
	if method == "" {
		method = "GET"
	}

	// Set endpoint
	endpoint := a.Path
	if endpoint == "" {
		endpoint = "/"
	}

	// Determine endpoint prefix
	var endpointPrefix string
	if a.Analytics {
		endpointPrefix = fmt.Sprintf("v2/analytics/organizations/%s", f.Config.OrganizationSlug())
	} else {
		endpointPrefix = fmt.Sprintf("v2/organizations/%s", f.Config.OrganizationSlug())
	}

	fullEndpoint := endpointPrefix + endpoint

	// Create HTTP client
	client := httpClient.NewClient(
		f.Config.APIToken(),
		httpClient.WithBaseURL(f.RestAPIClient.BaseURL.String()),
	)

	// Process custom headers
	customHeaders := a.Header.Values
	if customHeaders == nil {
		customHeaders = make(map[string]string)
	}

	// Prepare request data
	var requestData interface{}
	if a.Data != "" {
		// Try to parse as JSON first
		if err := json.Unmarshal([]byte(a.Data), &requestData); err != nil {
			// If not JSON, use raw string
			requestData = a.Data
		}
	}

	var response interface{}
	var err error

	// Make the request
	switch method {
	case "GET":
		err = client.DoWithHeaders(ctx, "GET", fullEndpoint, nil, &response, customHeaders)
	case "POST":
		err = client.DoWithHeaders(ctx, "POST", fullEndpoint, requestData, &response, customHeaders)
	case "PUT":
		err = client.DoWithHeaders(ctx, "PUT", fullEndpoint, requestData, &response, customHeaders)
	default:
		err = client.DoWithHeaders(ctx, method, fullEndpoint, requestData, &response, customHeaders)
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
