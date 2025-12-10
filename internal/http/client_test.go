package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
}
