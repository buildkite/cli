package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buildkite/roko"
)

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	StatusCode int
	Status     string
	URL        string
	Body       []byte
	Headers    http.Header
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

	maxRetries    int
	maxRetryDelay time.Duration
	onRetry       OnRetryFunc
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

// WithMaxRetries sets the maximum number of retries for rate-limited requests.
func WithMaxRetries(n int) ClientOption {
	return func(c *Client) {
		c.maxRetries = n
	}
}

// WithMaxRetryDelay sets the maximum delay between retries
func WithMaxRetryDelay(d time.Duration) ClientOption {
	return func(c *Client) {
		c.maxRetryDelay = d
	}
}

// WithOnRetry sets a callback that is invoked before each retry sleep.
func WithOnRetry(f OnRetryFunc) ClientOption {
	return func(c *Client) {
		c.onRetry = f
	}
}

// OnRetryFunc is called before each retry sleep with the attempt number and delay duration.
type OnRetryFunc func(attempt int, delay time.Duration)

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

// IsTooManyRequests returns true if the error is a 429 Too Many Requests
func (e *ErrorResponse) IsTooManyRequests() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

// RetryAfter returns the duration to wait before retrying, based on the RateLimit-Reset header.
// Returns 0 if the header is missing or invalid.
func (e *ErrorResponse) RetryAfter() time.Duration {
	if e.Headers == nil {
		return 0
	}
	resetStr := e.Headers.Get("RateLimit-Reset")
	if resetStr == "" {
		return 0
	}
	seconds, err := strconv.Atoi(resetStr)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// Do performs an HTTP request with the given method, endpoint, and body.
func (c *Client) Do(ctx context.Context, method, endpoint string, body interface{}, v interface{}) error {
	// Ensure endpoint starts with "/"
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	// Parse the endpoint to properly handle path, query string, and fragments
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	// Create the request URL using only the path portion
	reqURL, err := url.JoinPath(c.baseURL, parsedEndpoint.Path)
	if err != nil {
		return fmt.Errorf("failed to create request URL: %w", err)
	}

	// Reattach query string if present (properly encoded)
	if parsedEndpoint.RawQuery != "" {
		reqURL += "?" + parsedEndpoint.RawQuery
	}

	var bodyBytes []byte
	if body != nil {
		// We need to nest this in a branch because otherwise
		// `json.Marshal(nil)` produces `null` instead of `nil`.
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	r := roko.NewRetrier(
		roko.WithMaxAttempts(c.maxRetries+1),
		roko.WithStrategy(roko.Constant(0)),
	)

	respBody, err := roko.DoFunc(ctx, r, func(r *roko.Retrier) ([]byte, error) {
		resp, err := c.send(ctx, method, reqURL, bodyBytes)
		if err != nil {
			if !c.handleRetry(r, err) {
				r.Break()
			}
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return err
	}

	if v != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, v); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *Client) send(ctx context.Context, method, reqURL string, body []byte) ([]byte, error) {
	// Create the request
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status
	if resp.StatusCode >= 400 {
		return nil, &ErrorResponse{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        reqURL,
			Body:       respBody,
			Headers:    resp.Header,
		}
	}

	return respBody, nil
}

// handleRetry checks if an error is retryable and configures the retrier accordingly.
// Returns true if the request should be retried, false otherwise.
func (c *Client) handleRetry(r *roko.Retrier, err error) bool {
	errResp, ok := err.(*ErrorResponse)
	if !ok || !errResp.IsTooManyRequests() {
		return false
	}

	attempt := r.AttemptCount()
	delay := errResp.RetryAfter()
	if attempt > 0 {
		// Got rate-limited again means contention - back off exponentially
		delay *= time.Duration(1 << attempt)
	}

	if c.maxRetryDelay > 0 {
		delay = min(delay, c.maxRetryDelay)
	}

	if c.onRetry != nil {
		c.onRetry(attempt, delay)
	}

	r.SetNextInterval(delay)
	return true
}
