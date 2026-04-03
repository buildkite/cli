package factory

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
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

func TestNewWithoutAPIClientsDoesNotRefreshTokens(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			Error:     "invalid_grant",
			ErrorDesc: "should not be called",
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         server.URL,
		ClientID:     "stored-client",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	f, err := New(WithoutAPIClients())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if f.Config == nil {
		t.Fatal("expected config to be initialised")
	}
	if f.RestAPIClient != nil {
		t.Fatal("expected RestAPIClient to be nil when API clients are disabled")
	}
	if requests != 0 {
		t.Fatalf("refresh requests = %d, want 0", requests)
	}
}

func TestNewWithOrgOverrideRefreshesOnlyOverrideOrg(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "current-org")
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	currentRequests := 0
	currentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentRequests++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			Error:     "invalid_grant",
			ErrorDesc: "should not be called",
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer currentServer.Close()

	overrideRequests := 0
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		overrideRequests++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			AccessToken:  "bkua_override_access",
			RefreshToken: "bkrt_override_refresh",
			TokenType:    "Bearer",
			Scope:        "read_user",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer overrideServer.Close()

	kr := keyring.New()
	if err := kr.SetSession("current-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         currentServer.URL,
		ClientID:     "current-client",
		AccessToken:  "bkua_current_access",
		RefreshToken: "bkrt_current_refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession current-org returned error: %v", err)
	}
	if err := kr.SetSession("override-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         overrideServer.URL,
		ClientID:     "override-client",
		AccessToken:  "bkua_old_override_access",
		RefreshToken: "bkrt_old_override_refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession override-org returned error: %v", err)
	}

	f, err := New(WithOrgOverride("override-org"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if f.Token != "bkua_override_access" {
		t.Fatalf("Factory token = %q, want bkua_override_access", f.Token)
	}
	if currentRequests != 0 {
		t.Fatalf("current-org refresh requests = %d, want 0", currentRequests)
	}
	if overrideRequests != 1 {
		t.Fatalf("override-org refresh requests = %d, want 1", overrideRequests)
	}
}

func TestNewWithOrgOverrideDoesNotFallbackWhenOverrideOrgCredentialsAreExpired(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "current-org")
	t.Setenv("BUILDKITE_API_TOKEN", "")

	kr := keyring.New()
	if err := kr.SetSession("current-org", &oauth.Session{
		Version:     oauth.SessionVersion,
		Host:        "buildkite.localhost",
		AccessToken: "bkua_current_access",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SetSession current-org returned error: %v", err)
	}
	if err := kr.SetSession("override-org", &oauth.Session{
		Version:     oauth.SessionVersion,
		Host:        "buildkite.localhost",
		AccessToken: "bkua_expired_override_access",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession override-org returned error: %v", err)
	}

	f, err := New(WithOrgOverride("override-org"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if f.Token != "" {
		t.Fatalf("Factory token = %q, want empty token for expired override credentials", f.Token)
	}
}
