package errors

import (
	"encoding/json"
	"testing"

	httpClient "github.com/buildkite/cli/v3/internal/http"
)

func TestWrapAPIError(t *testing.T) {
	t.Parallel()

	t.Run("handles nil error", func(t *testing.T) {
		t.Parallel()
		result := WrapAPIError(nil, "test operation")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("preserves CLI error category", func(t *testing.T) {
		t.Parallel()
		original := NewValidationError(nil, "Invalid input")
		result := WrapAPIError(original, "test operation")

		if !IsValidationError(result) {
			t.Error("Expected validation error category to be preserved")
		}
	})

	t.Run("adds operation context to CLI error", func(t *testing.T) {
		t.Parallel()
		original := NewValidationError(nil, "Invalid input")
		result := WrapAPIError(original, "test operation")

		cliErr, ok := result.(*Error)
		if !ok {
			t.Fatal("Expected result to be a *Error")
		}

		if cliErr.Details != "test operation: Invalid input" {
			t.Errorf("Expected details to include operation, got: %q", cliErr.Details)
		}
	})

	t.Run("wraps generic error as API error", func(t *testing.T) {
		t.Parallel()
		original := &simpleError{message: "something went wrong"}
		result := WrapAPIError(original, "test operation")

		if !IsAPIError(result) {
			t.Error("Expected generic error to be wrapped as API error")
		}
	})
}

func TestHandleHTTPError(t *testing.T) {
	t.Parallel()

	t.Run("handles 404 not found", func(t *testing.T) {
		t.Parallel()
		httpErr := &httpClient.ErrorResponse{
			StatusCode: 404,
			Status:     "Not Found",
			URL:        "https://api.buildkite.com/v2/pipelines/missing",
			Body:       []byte(`{"message":"Pipeline not found"}`),
		}

		result := handleHTTPError(httpErr, "get pipeline")

		if !IsNotFound(result) {
			t.Error("Expected result to be a not found error")
		}

		// Check for pipeline-specific suggestion
		cliErr, ok := result.(*Error)
		if !ok {
			t.Fatal("Expected result to be a *Error")
		}

		foundSuggestion := false
		for _, suggestion := range cliErr.Suggestions {
			if suggestion == "Verify the pipeline slug is correct" {
				foundSuggestion = true
				break
			}
		}

		if !foundSuggestion {
			t.Error("Expected pipeline-specific suggestion for not found error")
		}
	})

	t.Run("handles 401 unauthorized", func(t *testing.T) {
		t.Parallel()
		httpErr := &httpClient.ErrorResponse{
			StatusCode: 401,
			Status:     "Unauthorized",
			URL:        "https://api.buildkite.com/v2/user",
			Body:       []byte(`{"message":"Unauthorized"}`),
		}

		result := handleHTTPError(httpErr, "get user")

		if !IsAuthenticationError(result) {
			t.Error("Expected result to be an authentication error")
		}

		// Check for token suggestion
		cliErr, ok := result.(*Error)
		if !ok {
			t.Fatal("Expected result to be a *Error")
		}

		foundSuggestion := false
		for _, suggestion := range cliErr.Suggestions {
			if suggestion == "Check your API token in the configuration" {
				foundSuggestion = true
				break
			}
		}

		if !foundSuggestion {
			t.Error("Expected token-specific suggestion for unauthorized error")
		}
	})

	t.Run("handles 403 forbidden", func(t *testing.T) {
		t.Parallel()
		httpErr := &httpClient.ErrorResponse{
			StatusCode: 403,
			Status:     "Forbidden",
			URL:        "https://api.buildkite.com/v2/builds",
			Body:       []byte(`{"message":"Forbidden"}`),
		}

		result := handleHTTPError(httpErr, "cancel build")

		if !IsPermissionDeniedError(result) {
			t.Error("Expected result to be a permission denied error")
		}
	})

	t.Run("handles 400 bad request with field errors", func(t *testing.T) {
		t.Parallel()
		apiErr := APIErrorResponse{
			Message: "Invalid request",
			Details: map[string]string{
				"name": "cannot be blank",
				"url":  "invalid format",
			},
		}

		body, _ := json.Marshal(apiErr)
		httpErr := &httpClient.ErrorResponse{
			StatusCode: 400,
			Status:     "Bad Request",
			URL:        "https://api.buildkite.com/v2/pipelines",
			Body:       body,
		}

		result := handleHTTPError(httpErr, "create pipeline")

		if !IsValidationError(result) {
			t.Error("Expected result to be a validation error")
		}

		// Check that field errors are included in suggestions
		cliErr, ok := result.(*Error)
		if !ok {
			t.Fatal("Expected result to be a *Error")
		}

		foundNameError := false
		foundURLError := false
		for _, suggestion := range cliErr.Suggestions {
			if suggestion == "name: cannot be blank" {
				foundNameError = true
			}
			if suggestion == "url: invalid format" {
				foundURLError = true
			}
		}

		if !foundNameError || !foundURLError {
			t.Error("Expected field-specific errors to be included in suggestions")
		}
	})

	t.Run("handles 500 server error", func(t *testing.T) {
		t.Parallel()
		httpErr := &httpClient.ErrorResponse{
			StatusCode: 500,
			Status:     "Internal Server Error",
			URL:        "https://api.buildkite.com/v2/builds",
			Body:       []byte(`{"message":"Internal server error"}`),
		}

		result := handleHTTPError(httpErr, "list builds")

		if !IsAPIError(result) {
			t.Error("Expected result to be an API error")
		}

		// Check for server error suggestion
		cliErr, ok := result.(*Error)
		if !ok {
			t.Fatal("Expected result to be a *Error")
		}

		foundSuggestion := false
		for _, suggestion := range cliErr.Suggestions {
			if suggestion == "This appears to be a server-side error" {
				foundSuggestion = true
				break
			}
		}

		if !foundSuggestion {
			t.Error("Expected server error suggestion")
		}
	})
}

// simpleError is a simple implementation of the error interface for testing
type simpleError struct {
	message string
}

func (e *simpleError) Error() string {
	return e.message
}
