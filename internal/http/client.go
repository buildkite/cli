// Package http provides a common HTTP client with standardized headers and error handling
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	StatusCode int
	Status     string
	URL        string
	Body       []byte
}

// Error implements the error interface
func (e *ErrorResponse) Error() string {
	msg := fmt.Sprintf("HTTP request failed: %d %s (%s)", e.StatusCode, e.Status, e.URL)
	if len(e.Body) > 0 {
		// Truncate body if it's very long for the error message
		bodyStr := string(e.Body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		msg += fmt.Sprintf(": %s", bodyStr)
	}
	return msg
}

// Client is an HTTP client that handles common operations for Buildkite API requests
type Client struct {
	baseURL   string
	token     string
	userAgent string
	client    *http.Client
}

// ClientOption is a function that modifies a Client
type ClientOption func(*Client)

// WithBaseURL sets the base URL for API requests
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithUserAgent sets the User-Agent header for requests
func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

// WithHTTPClient sets the underlying HTTP client
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.client = client
	}
}

// NewClient creates a new HTTP client with the given token and options
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:   "https://api.buildkite.com",
		token:     token,
		userAgent: "buildkite-cli",
		client:    http.DefaultClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Get performs a GET request to the specified endpoint
func (c *Client) Get(ctx context.Context, endpoint string, v interface{}) error {
	return c.Do(ctx, http.MethodGet, endpoint, nil, v)
}

// Post performs a POST request to the specified endpoint with the given body
func (c *Client) Post(ctx context.Context, endpoint string, body interface{}, v interface{}) error {
	return c.Do(ctx, http.MethodPost, endpoint, body, v)
}

// Put performs a PUT request to the specified endpoint with the given body
func (c *Client) Put(ctx context.Context, endpoint string, body interface{}, v interface{}) error {
	return c.Do(ctx, http.MethodPut, endpoint, body, v)
}

// Delete performs a DELETE request to the specified endpoint
func (c *Client) Delete(ctx context.Context, endpoint string, v interface{}) error {
	return c.Do(ctx, http.MethodDelete, endpoint, nil, v)
}

// IsNotFound returns true if the error is a 404 Not Found
func (e *ErrorResponse) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsUnauthorized returns true if the error is a 401 Unauthorized
func (e *ErrorResponse) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// IsForbidden returns true if the error is a 403 Forbidden
func (e *ErrorResponse) IsForbidden() bool {
	return e.StatusCode == http.StatusForbidden
}

// IsBadRequest returns true if the error is a 400 Bad Request
func (e *ErrorResponse) IsBadRequest() bool {
	return e.StatusCode == http.StatusBadRequest
}

// IsServerError returns true if the error is a 5xx Server Error
func (e *ErrorResponse) IsServerError() bool {
	return e.StatusCode >= 500
}

// Do performs an HTTP request with the given method, endpoint, and body
func (c *Client) Do(ctx context.Context, method, endpoint string, body interface{}, v interface{}) error {
	// Ensure endpoint starts with "/"
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	// Create the request URL
	reqURL, err := url.JoinPath(c.baseURL, endpoint)
	if err != nil {
		return fmt.Errorf("failed to create request URL: %w", err)
	}

	// Create the request body
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set common headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status
	if resp.StatusCode >= 400 {
		return &ErrorResponse{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        reqURL,
			Body:       respBody,
		}
	}

	// Parse the response if a target was provided
	if v != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, v); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	StatusCode int
	Status     string
	URL        string
	Body       []byte
}

// Error implements the error interface
func (e *ErrorResponse) Error() string {
	msg := fmt.Sprintf("HTTP request failed: %d %s (%s)", e.StatusCode, e.Status, e.URL)
	if len(e.Body) > 0 {
		msg += fmt.Sprintf(": %s", e.Body)
	}
	return msg
}
