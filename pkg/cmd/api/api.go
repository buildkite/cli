package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

var (
	method      string
	headers     []string
	data        string
	baseURL     string
	graphqlFlag bool
	queryFile   string
)

func NewCmdAPI(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "api <endpoint>",
		Short: "Interact with the Buildkite API",
		Long:  "Interact with either the REST or GraphQL Buildkite APIs",
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
      # To make an API call
      $ bk api pipelines/example-pipeline/builds/420
      `),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		RunE: func(cmd *cobra.Command, args []string) error {
			return apiCaller(cmd, args, f)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", "GET", "HTTP method to use")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", []string{}, "Headers to include in the request")
	cmd.Flags().StringVarP(&data, "data", "d", "", "Data to send in the request body")
	cmd.Flags().StringVar(&baseURL, "base-url", fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s", f.Config.OrganizationSlug()), "Base URL for the API")
	cmd.Flags().BoolVar(&graphqlFlag, "graphql", false, "Use GraphQL API")
	cmd.Flags().StringVarP(&queryFile, "file", "f", "", "File containing GraphQL query")

	return &cmd
}

func apiCaller(cmd *cobra.Command, args []string, f *factory.Factory) error {
	var endpoint string

	if len(args) > 1 {
		return fmt.Errorf("Incorrect numebr of arguments. Expected 1, got %d", len(args))
	}

	if len(args) == 0 {
		endpoint = "/"
	} else {
		endpoint = args[0]
	}
	url := baseURL + endpoint

	var req *http.Request
	var err error

	if graphqlFlag {
		return errors.New("GraphQL not supported at this time.")
	}

	if data != "" {
		req, err = http.NewRequest(method, url, strings.NewReader(data))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	for _, header := range headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	req.Header.Set("Authorization", "Bearer "+f.Config.APIToken())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	var jsonResponse interface{}
	err = json.Unmarshal(body, &jsonResponse)
	if err != nil {
		return fmt.Errorf("error parsing JSON response: %w", err)
	}

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, body, "", "  ")
	if err != nil {
		fmt.Println(string(body))
		return fmt.Errorf("error parsing JSON response: %w", err)
	}

	fmt.Println(prettyJSON.String())

	return nil
}
