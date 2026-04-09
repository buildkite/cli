package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAccessToken_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/oauth/token" {
			t.Errorf("expected /oauth/token, got %s", r.URL.Path)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", got)
		}
		if got := r.FormValue("refresh_token"); got != "bkur_old_refresh_token" {
			t.Errorf("expected refresh_token=bkur_old_refresh_token, got %s", got)
		}
		if got := r.FormValue("client_id"); got != "test-client" {
			t.Errorf("expected client_id=test-client, got %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"access_token": "new_access_token",
			"token_type": "Bearer",
			"scope": "read_user read_organizations",
			"refresh_token": "bkur_new_refresh_token",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	// Override the default HTTP client to trust the test server's TLS cert
	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	// Extract host from the test server URL (strip https://)
	host := server.URL[len("https://"):]

	resp, err := RefreshAccessToken(context.Background(), host, "test-client", "bkur_old_refresh_token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AccessToken != "new_access_token" {
		t.Errorf("expected access_token=new_access_token, got %s", resp.AccessToken)
	}
	if resp.RefreshToken != "bkur_new_refresh_token" {
		t.Errorf("expected refresh_token=bkur_new_refresh_token, got %s", resp.RefreshToken)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected expires_in=3600, got %d", resp.ExpiresIn)
	}
	if resp.Scope != "read_user read_organizations" {
		t.Errorf("expected scope=read_user read_organizations, got %s", resp.Scope)
	}
}

func TestRefreshAccessToken_ErrorResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
			"error": "invalid_grant",
			"error_description": "Invalid refresh token"
		}`))
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	host := server.URL[len("https://"):]

	_, err := RefreshAccessToken(context.Background(), host, "test-client", "bad-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expected := "token refresh error: invalid_grant - Invalid refresh token"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}
