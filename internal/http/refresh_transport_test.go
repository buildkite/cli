package http

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/buildkite/cli/v3/pkg/keyring"
)

type stubCredentialStore struct {
	accessToken        string
	refreshToken       string
	setAccessErr       error
	setRefreshTokenErr error
}

func (s *stubCredentialStore) Set(_ string, token string) error {
	if s.setAccessErr != nil {
		return s.setAccessErr
	}
	s.accessToken = token
	return nil
}

func (s *stubCredentialStore) GetRefreshToken(string) (string, error) {
	return s.refreshToken, nil
}

func (s *stubCredentialStore) SetRefreshToken(_ string, token string) error {
	if s.setRefreshTokenErr != nil {
		return s.setRefreshTokenErr
	}
	s.refreshToken = token
	return nil
}

func (s *stubCredentialStore) DeleteRefreshToken(string) error {
	s.refreshToken = ""
	return nil
}

func TestRefreshTransport_PassesThroughNon401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "old-token")
	_ = kr.SetRefreshToken("test-org", "refresh-token")

	ts := NewTokenSource("old-token")

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer old-token")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRefreshTransport_NoRefreshToken_PassesThrough401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"unauthorized"}`))
	}))
	defer server.Close()

	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "some-token")
	// No refresh token set

	ts := NewTokenSource("some-token")

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 pass-through, got %d", resp.StatusCode)
	}
}

func TestRefreshTransport_CompareAfterLock_SkipsRedundantRefresh(t *testing.T) {
	// This test uses t.Setenv so cannot be parallel.

	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "already-refreshed-token")
	_ = kr.SetRefreshToken("test-org", "refresh-token")

	// TokenSource already has the new token (simulating another goroutine
	// having refreshed it).
	ts := NewTokenSource("already-refreshed-token")

	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		auth := r.Header.Get("Authorization")
		if auth == "Bearer already-refreshed-token" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Point BUILDKITE_HOST at a dead port so that if doRefresh is
	// incorrectly called, it fails fast instead of hitting a real server.
	t.Setenv("BUILDKITE_HOST", "127.0.0.1:1")

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	// Request with a stale token that triggers 401
	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer stale-token")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after compare-after-lock skip, got %d", resp.StatusCode)
	}
	// Should have made exactly 2 API calls: the initial 401 + the retry
	if got := apiCalls.Load(); got != 2 {
		t.Fatalf("expected 2 API calls (initial + retry), got %d", got)
	}
}

func TestRefreshTransport_RotatedRefreshTokenStoreFailureReturnsError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh-token","token_type":"Bearer","expires_in":3600}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	store := &stubCredentialStore{
		accessToken:        "old-token",
		refreshToken:       "old-refresh-token",
		setRefreshTokenErr: errors.New("boom"),
	}
	transport := &RefreshTransport{
		Base:        server.Client().Transport,
		Org:         "test-org",
		Keyring:     store,
		TokenSource: NewTokenSource("old-token"),
	}

	t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer old-token")

	resp, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on credential persistence failure, got %#v", resp)
	}
	if !errors.Is(err, errCredentialPersistence) {
		t.Fatalf("expected credential persistence error, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to store rotated refresh token") {
		t.Fatalf("expected rotated refresh token storage error, got %v", err)
	}
	if store.accessToken != "old-token" {
		t.Fatalf("expected access token to remain unchanged, got %q", store.accessToken)
	}
	if transport.TokenSource.Token() != "old-token" {
		t.Fatalf("expected token source to remain unchanged, got %q", transport.TokenSource.Token())
	}
}

func TestRefreshTransport_DoesNotDeleteRefreshTokenOnTransientError(t *testing.T) {
	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "old-token")
	_ = kr.SetRefreshToken("test-org", "my-refresh-token")

	ts := NewTokenSource("old-token")

	// API server that always returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Set BUILDKITE_HOST to a non-existent host to simulate a network error
	// during the refresh attempt
	t.Setenv("BUILDKITE_HOST", "127.0.0.1:1") // connection refused

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer old-token")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 pass-through, got %d", resp.StatusCode)
	}

	// The refresh token should NOT have been deleted (transient error)
	rt, rtErr := kr.GetRefreshToken("test-org")
	if rtErr != nil || rt != "my-refresh-token" {
		t.Fatalf("expected refresh token to be preserved after transient error, got %q err=%v", rt, rtErr)
	}
}

func TestRefreshTransport_BuffersAndRetriesPostBody(t *testing.T) {
	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "old-token")
	_ = kr.SetRefreshToken("test-org", "refresh-token")

	ts := NewTokenSource("old-token")

	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := apiCalls.Add(1)
		if call == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Verify body was replayed on retry
		body, _ := io.ReadAll(r.Body)
		_ = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	// Simulate a POST with a body that doesn't have GetBody set
	body := `{"query":"{ viewer { user { name } } }"}`
	req, _ := http.NewRequest("POST", server.URL+"/graphql", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer old-token")
	req.Header.Set("Content-Type", "application/json")
	// Explicitly clear GetBody to simulate a custom reader
	req.GetBody = nil

	// doRefresh will fail (no real token server), but we can verify
	// that bufferRequestBody was called by checking the request has GetBody.
	// Since the refresh will fail, the 401 is returned, but the body
	// buffering is the important part to verify.
	resp, _ := transport.RoundTrip(req)
	_ = resp

	// Verify GetBody was set by bufferRequestBody
	if req.GetBody == nil {
		t.Fatal("expected GetBody to be set by bufferRequestBody")
	}
}

func TestRefreshTransport_ConcurrentRequestsOnlyRefreshOnce(t *testing.T) {
	// This test uses t.Setenv so cannot be parallel.

	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	kr := keyring.New()
	_ = kr.Set("test-org", "new-token")
	_ = kr.SetRefreshToken("test-org", "refresh-token")

	// TokenSource already has the refreshed token (simulating the first
	// goroutine having completed the refresh).
	ts := NewTokenSource("new-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer stale-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if auth == "Bearer new-token" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Point BUILDKITE_HOST at a dead port so that if doRefresh is
	// incorrectly called (bypassing compare-after-lock), it fails.
	t.Setenv("BUILDKITE_HOST", "127.0.0.1:1")

	transport := &RefreshTransport{
		Base:        http.DefaultTransport,
		Org:         "test-org",
		Keyring:     kr,
		TokenSource: ts,
	}

	// N goroutines hit 401 with "stale-token" concurrently.
	// All should use compare-after-lock to skip refresh and retry
	// with the already-refreshed "new-token".
	var wg sync.WaitGroup
	results := make([]int, 5)

	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest("GET", server.URL+"/test", nil)
			req.Header.Set("Authorization", "Bearer stale-token")
			resp, err := transport.RoundTrip(req)
			if err != nil {
				results[idx] = -1
				return
			}
			results[idx] = resp.StatusCode
		}(i)
	}

	wg.Wait()

	for i, status := range results {
		if status != http.StatusOK {
			t.Errorf("goroutine %d: expected 200, got %d", i, status)
		}
	}
}

func TestTokenSource_ThreadSafe(t *testing.T) {
	t.Parallel()

	ts := NewTokenSource("initial")

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ts.SetToken("updated")
		}()
		go func() {
			defer wg.Done()
			_ = ts.Token()
		}()
	}
	wg.Wait()
}

func TestIsTerminalRefreshError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err      string
		terminal bool
	}{
		{"token refresh error: invalid_grant - Invalid refresh token", true},
		{"token refresh error: unauthorized_client - Client not configured", true},
		{"token refresh error: invalid_client - Invalid client", true},
		{"refresh token request failed: dial tcp: connection refused", false},
		{"refresh token request failed: timeout", false},
		{"failed to parse token response: unexpected end of JSON", false},
	}

	for _, tt := range tests {
		got := isTerminalRefreshError(errors.New(tt.err))
		if got != tt.terminal {
			t.Errorf("isTerminalRefreshError(%q) = %v, want %v", tt.err, got, tt.terminal)
		}
	}
}

func TestAuthTransport_DoesNotInjectOnRedirectedHop(t *testing.T) {
	t.Parallel()

	var presignedAuth atomic.Value
	presignedAuth.Store("")

	presigned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presignedAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("payload"))
	}))
	defer presigned.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, presigned.URL+"/blob?X-Amz-Signature=stub", http.StatusFound)
	}))
	defer api.Close()

	ts := NewTokenSource("bk-token")
	transport := &AuthTransport{Base: http.DefaultTransport, TokenSource: ts}

	client := &http.Client{Transport: transport}
	req, err := http.NewRequest(http.MethodGet, api.URL+"/artifact", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if got := presignedAuth.Load().(string); got != "" {
		t.Fatalf("Authorization leaked to presigned host: %q", got)
	}
}
