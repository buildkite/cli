package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type testResponse struct {
	Message string `json:"message"`
}

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("makes request with authorization header", func(t *testing.T) {
		t.Parallel()

		var receivedToken string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedToken = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedToken := "Bearer test-token"
		if receivedToken != expectedToken {
			t.Errorf("expected Authorization header %q, got %q", expectedToken, receivedToken)
		}

		if resp.Message != "success" {
			t.Errorf("expected response message %q, got %q", "success", resp.Message)
		}
	})

	t.Run("handles JSON response", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testResponse{Message: "test message"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Message != "test message" {
			t.Errorf("expected message %q, got %q", "test message", resp.Message)
		}
	})

	t.Run("handles error response", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		// Error should contain status code and possibly the error message
		if errStr := err.Error(); errStr == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("adds user agent header", func(t *testing.T) {
		t.Parallel()

		var receivedUA string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUA = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		expectedUA := "test-user-agent"
		client := NewClient("test-token", WithBaseURL(server.URL), WithUserAgent(expectedUA))

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedUA != expectedUA {
			t.Errorf("expected User-Agent header %q, got %q", expectedUA, receivedUA)
		}
	})

	t.Run("handles POST request with body", func(t *testing.T) {
		t.Parallel()

		var receivedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			receivedBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		requestBody := map[string]string{"test": "data"}
		var resp testResponse
		err := client.Post(context.Background(), "/test", requestBody, &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that the body was correctly serialized
		var parsed map[string]string
		if err := json.Unmarshal(receivedBody, &parsed); err != nil {
			t.Fatalf("failed to parse received body: %v", err)
		}
		if parsed["test"] != "data" {
			t.Errorf("expected body to contain %q, got %q", "data", parsed["test"])
		}
	})

	t.Run("preserves query parameters in endpoint", func(t *testing.T) {
		t.Parallel()

		var receivedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/builds?branch=main&state=passed", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedQuery := "branch=main&state=passed"
		if receivedQuery != expectedQuery {
			t.Errorf("expected query string %q, got %q", expectedQuery, receivedQuery)
		}
	})

	t.Run("handles encoded query parameters", func(t *testing.T) {
		t.Parallel()

		var receivedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/builds?branch=feature%2Ftest%20name", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedQuery := "branch=feature%2Ftest%20name"
		if receivedQuery != expectedQuery {
			t.Errorf("expected query string %q, got %q", expectedQuery, receivedQuery)
		}
	})

	t.Run("strips fragments from endpoint", func(t *testing.T) {
		t.Parallel()

		var receivedPath string
		var receivedQuery string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			receivedQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/builds?branch=main#foo", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedPath != "/builds" {
			t.Errorf("expected path %q, got %q", "/builds", receivedPath)
		}
		expectedQuery := "branch=main"
		if receivedQuery != expectedQuery {
			t.Errorf("expected query string %q, got %q", expectedQuery, receivedQuery)
		}
	})

	t.Run("handles endpoint without query parameters", func(t *testing.T) {
		t.Parallel()

		var receivedPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token", WithBaseURL(server.URL))

		var resp testResponse
		err := client.Get(context.Background(), "/pipelines/test/builds", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedPath := "/pipelines/test/builds"
		if receivedPath != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, receivedPath)
		}
	})
}

func TestErrorResponse(t *testing.T) {
	t.Parallel()

	t.Run("formats status code errors", func(t *testing.T) {
		t.Parallel()

		err := &ErrorResponse{
			StatusCode: 404,
			Status:     "Not Found",
			URL:        "https://example.com/resource",
		}

		expected := "HTTP request failed: 404 Not Found (https://example.com/resource)"
		if err.Error() != expected {
			t.Errorf("expected error message %q, got %q", expected, err.Error())
		}
	})

	t.Run("includes body in error", func(t *testing.T) {
		t.Parallel()

		err := &ErrorResponse{
			StatusCode: 400,
			Status:     "Bad Request",
			URL:        "https://example.com/resource",
			Body:       []byte(`{"error":"Invalid input"}`),
		}

		expected := "HTTP request failed: 400 Bad Request (https://example.com/resource): {\"error\":\"Invalid input\"}"
		if err.Error() != expected {
			t.Errorf("expected error message %q, got %q", expected, err.Error())
		}
	})

	t.Run("IsTooManyRequests returns true for 429", func(t *testing.T) {
		t.Parallel()

		err := &ErrorResponse{StatusCode: 429}
		if !err.IsTooManyRequests() {
			t.Error("expected IsTooManyRequests to return true for 429")
		}

		err = &ErrorResponse{StatusCode: 500}
		if err.IsTooManyRequests() {
			t.Error("expected IsTooManyRequests to return false for 500")
		}
	})

	t.Run("RetryAfter parses RateLimit-Reset header", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{}
		headers.Set("RateLimit-Reset", "30")
		err := &ErrorResponse{Headers: headers}

		if got := err.RetryAfter(); got != 30*time.Second {
			t.Errorf("expected 30s, got %v", got)
		}
	})

	t.Run("RetryAfter returns zero for missing header", func(t *testing.T) {
		t.Parallel()

		err := &ErrorResponse{Headers: http.Header{}}
		if got := err.RetryAfter(); got != 0 {
			t.Errorf("expected 0, got %v", got)
		}

		err = &ErrorResponse{Headers: nil}
		if got := err.RetryAfter(); got != 0 {
			t.Errorf("expected 0 for nil headers, got %v", got)
		}
	})

	t.Run("RetryAfter returns zero for invalid header value", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{}
		headers.Set("RateLimit-Reset", "not-a-number")
		err := &ErrorResponse{Headers: headers}

		if got := err.RetryAfter(); got != 0 {
			t.Errorf("expected 0 for invalid value, got %v", got)
		}
	})
}

func TestClientRetry(t *testing.T) {
	t.Parallel()

	t.Run("retries on 429 with RateLimit-Reset header", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count == 1 {
				w.Header().Set("RateLimit-Reset", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(3),
			WithMaxRetryDelay(100*time.Millisecond),
		)

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := requestCount.Load(); got != 2 {
			t.Errorf("expected 2 requests, got %d", got)
		}
		if resp.Message != "success" {
			t.Errorf("expected success message, got %q", resp.Message)
		}
	})

	t.Run("respects max retries limit", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.Header().Set("RateLimit-Reset", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(2),
			WithMaxRetryDelay(1*time.Millisecond),
		)

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		errResp, ok := err.(*ErrorResponse)
		if !ok {
			t.Fatalf("expected ErrorResponse, got %T", err)
		}
		if !errResp.IsTooManyRequests() {
			t.Errorf("expected 429 error, got %d", errResp.StatusCode)
		}

		if got := requestCount.Load(); got != 3 {
			t.Errorf("expected 3 requests, got %d", got)
		}
	})

	t.Run("does not retry non-429 errors", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(3),
		)

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if got := requestCount.Load(); got != 1 {
			t.Errorf("expected 1 request (no retries for 500), got %d", got)
		}
	})

	t.Run("respects context cancellation during retry", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("RateLimit-Reset", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(3),
			WithMaxRetryDelay(60*time.Second),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		var resp testResponse
		err := client.Get(ctx, "/test", &resp)

		elapsed := time.Since(start)
		if elapsed > 1*time.Second {
			t.Errorf("expected quick cancellation, took %v", elapsed)
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})

	t.Run("honors RateLimit-Reset header when no max retry delay is set", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("RateLimit-Reset", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var observedDelay time.Duration

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(1),
			WithOnRetry(func(attempt int, delay time.Duration) {
				// Stop the request as soon as we see the computed delay so the test
				// doesn't actually sleep for the full backoff.
				observedDelay = delay
				cancel()
			}),
		)

		var resp testResponse
		err := client.Get(ctx, "/test", &resp)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
		if observedDelay != time.Second {
			t.Fatalf("expected retry delay from RateLimit-Reset to be 1s, got %v", observedDelay)
		}
	})

	t.Run("caps retry delay at maxRetryDelay", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count == 1 {
				w.Header().Set("RateLimit-Reset", "3600")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(1),
			WithMaxRetryDelay(10*time.Millisecond),
		)

		start := time.Now()
		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed > 1*time.Second {
			t.Errorf("expected delay to be capped, but took %v", elapsed)
		}
	})

	t.Run("retries preserve request body", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		var lastBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			lastBody = body
			if count == 1 {
				w.Header().Set("RateLimit-Reset", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(1),
			WithMaxRetryDelay(1*time.Millisecond),
		)

		requestBody := map[string]string{"key": "value"}
		var resp testResponse
		err := client.Post(context.Background(), "/test", requestBody, &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]string
		if err := json.Unmarshal(lastBody, &parsed); err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		if parsed["key"] != "value" {
			t.Errorf("expected body to be preserved on retry, got %v", parsed)
		}
	})

	t.Run("invokes OnRetry callback before sleeping", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count <= 2 {
				w.Header().Set("RateLimit-Reset", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(testResponse{Message: "success"})
		}))
		defer server.Close()

		type callback struct {
			attempt int
			delay   time.Duration
		}
		var callbacks []callback

		client := NewClient("test-token",
			WithBaseURL(server.URL),
			WithMaxRetries(3),
			WithMaxRetryDelay(10*time.Millisecond),
			WithOnRetry(func(attempt int, delay time.Duration) {
				callbacks = append(callbacks, callback{attempt, delay})
			}),
		)

		var resp testResponse
		err := client.Get(context.Background(), "/test", &resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(callbacks) != 2 {
			t.Fatalf("expected 2 callbacks, got %d", len(callbacks))
		}
		if callbacks[0].attempt != 0 {
			t.Errorf("first callback attempt: expected 0, got %d", callbacks[0].attempt)
		}
		if callbacks[1].attempt != 1 {
			t.Errorf("second callback attempt: expected 1, got %d", callbacks[1].attempt)
		}
	})
}
