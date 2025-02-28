package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	httpClient "github.com/buildkite/cli/v3/internal/http"
)

// APIErrorResponse represents a Buildkite API error response
type APIErrorResponse struct {
	Message string            `json:"message"`
	Errors  []string          `json:"errors,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// WrapAPIError wraps an API error with appropriate context and suggestions
func WrapAPIError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// If it's already a CLI error, add context but preserve the category
	if cliErr, ok := err.(*Error); ok {
		if operation != "" {
			cliErr.Details = fmt.Sprintf("%s: %s", operation, cliErr.Details)
		}
		return cliErr
	}

	// Handle HTTP client errors
	if httpErr, ok := err.(*httpClient.ErrorResponse); ok {
		return handleHTTPError(httpErr, operation)
	}

	// For all other errors, wrap as a generic API error
	return NewAPIError(err, fmt.Sprintf("API request failed during: %s", operation))
}

// handleHTTPError processes an HTTP error and creates an appropriate CLI error
func handleHTTPError(httpErr *httpClient.ErrorResponse, operation string) error {
	statusCode := httpErr.StatusCode
	details := fmt.Sprintf("%s failed with status %d", operation, statusCode)
	
	// Try to parse the response body as JSON
	var apiErr APIErrorResponse
	if len(httpErr.Body) > 0 {
		if err := json.Unmarshal(httpErr.Body, &apiErr); err == nil {
			// Successfully parsed API error
			if apiErr.Message != "" {
				details = fmt.Sprintf("%s: %s", details, apiErr.Message)
			}
		} else {
			// If not JSON, include the raw body
			if len(httpErr.Body) > 0 {
				bodyStr := string(httpErr.Body)
				if len(bodyStr) > 200 {
					bodyStr = bodyStr[:200] + "..."
				}
				details = fmt.Sprintf("%s: %s", details, bodyStr)
			}
		}
	}

	// Create appropriate error based on status code
	var err error
	switch {
	case statusCode == http.StatusNotFound:
		err = NewResourceNotFoundError(httpErr, details, suggestForNotFound(httpErr.URL)...)
	case statusCode == http.StatusUnauthorized:
		err = NewAuthenticationError(httpErr, details, 
			"Check your API token in the configuration",
			"Run 'bk configure' to set up your token correctly")
	case statusCode == http.StatusForbidden:
		err = NewPermissionDeniedError(httpErr, details,
			"Verify that your API token has the required scopes",
			"You may need to create a new token with additional permissions")
	case statusCode == http.StatusBadRequest:
		err = handleBadRequestError(httpErr, details, apiErr)
	case statusCode >= 500:
		err = NewAPIError(httpErr, details,
			"This appears to be a server-side error",
			"Try again later or check the Buildkite status page")
	default:
		err = NewAPIError(httpErr, details)
	}

	return err
}

// handleBadRequestError processes a 400 Bad Request error
func handleBadRequestError(httpErr *httpClient.ErrorResponse, details string, apiErr APIErrorResponse) error {
	suggestions := []string{}
	
	// Add specific errors as suggestions
	if len(apiErr.Errors) > 0 {
		for _, errMsg := range apiErr.Errors {
			suggestions = append(suggestions, errMsg)
		}
	} else {
		suggestions = append(suggestions, "Check the request parameters for invalid values")
	}

	// Add field-specific errors
	if len(apiErr.Details) > 0 {
		for field, msg := range apiErr.Details {
			suggestions = append(suggestions, fmt.Sprintf("%s: %s", field, msg))
		}
	}

	return NewValidationError(httpErr, details, suggestions...)
}

// suggestForNotFound generates suggestions for a 404 Not Found error
func suggestForNotFound(url string) []string {
	suggestions := []string{
		"Check that the resource exists and you have access to it",
	}

	// Add more specific suggestions based on the URL pattern
	if strings.Contains(url, "/pipelines/") {
		suggestions = append(suggestions, "Verify the pipeline slug is correct")
	} else if strings.Contains(url, "/builds/") {
		suggestions = append(suggestions, "Verify the build number is correct")
	} else if strings.Contains(url, "/artifacts/") {
		suggestions = append(suggestions, "Verify the artifact ID is correct")
	} else if strings.Contains(url, "/agents/") {
		suggestions = append(suggestions, "Verify the agent ID is correct")
	}

	return suggestions
}
