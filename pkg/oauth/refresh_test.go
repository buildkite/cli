package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAccessToken(t *testing.T) {
	originalDefault := DefaultClientID
	DefaultClientID = ""
	t.Cleanup(func() {
		DefaultClientID = originalDefault
	})

	t.Setenv(EnvClientID, "env-client")
	t.Setenv(LegacyEnvClientID, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected token path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm returned error: %v", err)
		}

		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "bkrt_old_refresh" {
			t.Fatalf("refresh_token = %q, want bkrt_old_refresh", got)
		}
		if got := r.Form.Get("client_id"); got != "env-client" {
			t.Fatalf("client_id = %q, want env-client", got)
		}
		if got := r.Form.Get("scope"); got != "read_user" {
			t.Fatalf("scope = %q, want read_user", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "bkua_new_access",
			RefreshToken: "bkrt_new_refresh",
			TokenType:    "Bearer",
			Scope:        "read_user",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	tokenResp, err := RefreshAccessToken(context.Background(), &Config{Host: server.URL}, "bkrt_old_refresh", "read_user")
	if err != nil {
		t.Fatalf("RefreshAccessToken returned error: %v", err)
	}

	if tokenResp.AccessToken != "bkua_new_access" {
		t.Fatalf("AccessToken = %q, want bkua_new_access", tokenResp.AccessToken)
	}
	if tokenResp.RefreshToken != "bkrt_new_refresh" {
		t.Fatalf("RefreshToken = %q, want bkrt_new_refresh", tokenResp.RefreshToken)
	}
	if tokenResp.ExpiresIn != 3600 {
		t.Fatalf("ExpiresIn = %d, want 3600", tokenResp.ExpiresIn)
	}
}
