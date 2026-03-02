package factory

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		value    string
		expected string
	}{
		{
			name:     "Bearer token",
			header:   "Authorization",
			value:    "Bearer bkua_1234567890abcdef",
			expected: "Bearer [REDACTED]",
		},
		{
			name:     "Basic auth",
			header:   "Authorization",
			value:    "Basic dXNlcjpwYXNz",
			expected: "Basic [REDACTED]",
		},
		{
			name:     "Token without type",
			header:   "Authorization",
			value:    "sometoken123",
			expected: "[REDACTED]",
		},
		{
			name:     "Non-sensitive header unchanged",
			header:   "Content-Type",
			value:    "application/json",
			expected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set(tt.header, tt.value)

			redactHeaders(headers)

			got := headers.Get(tt.header)
			if got != tt.expected {
				t.Errorf("redactHeaders() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRedactHeadersMultipleValues(t *testing.T) {
	headers := http.Header{}
	headers.Add("Authorization", "Bearer token1")
	headers.Add("Authorization", "Bearer token2")

	redactHeaders(headers)

	values := headers.Values("Authorization")
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}

	for _, v := range values {
		if v != "Bearer [REDACTED]" {
			t.Errorf("expected 'Bearer [REDACTED]', got %q", v)
		}
	}
}

func TestDebugTransportPreservesRequestBody(t *testing.T) {
	expectedBody := `{"name":"test-pipeline","cluster_id":"","repository":"git@github.com:test/repo.git"}`

	// Create a test server that checks the request body.
	// Note: the handler runs in a separate goroutine, so we capture errors
	// in a variable rather than calling t.Fatalf (which would hang the test).
	var receivedBody string
	var handlerErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErr = err
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// Use the debug transport
	dt := &debugTransport{
		transport: http.DefaultTransport,
	}

	req, err := http.NewRequest("POST", server.URL, strings.NewReader(expectedBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if handlerErr != nil {
		t.Fatalf("handler failed to read request body: %v", handlerErr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if receivedBody != expectedBody {
		t.Errorf("request body was not preserved through debug transport\ngot:  %q\nwant: %q", receivedBody, expectedBody)
	}
}

func TestDebugTransportHandlesNilBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dt := &debugTransport{
		transport: http.DefaultTransport,
	}

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
