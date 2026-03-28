package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	"github.com/spf13/afero"
)

func TestAPITokenForOrgRefreshesStoredOAuthSession(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

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

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			AccessToken:  "bkua_refreshed_access",
			RefreshToken: "bkrt_rotated_refresh",
			TokenType:    "Bearer",
			Scope:        "read_user read_organizations",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	conf := New(afero.NewMemMapFs(), nil)
	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         server.URL,
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user read_organizations",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	token := conf.APITokenForOrg("test-org")
	if token != "bkua_refreshed_access" {
		t.Fatalf("APITokenForOrg() = %q, want bkua_refreshed_access", token)
	}
	if requests != 1 {
		t.Fatalf("refresh requests = %d, want 1", requests)
	}

	session, err := kr.GetSession("test-org")
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.AccessToken != "bkua_refreshed_access" {
		t.Fatalf("stored AccessToken = %q, want bkua_refreshed_access", session.AccessToken)
	}
	if session.RefreshToken != "bkrt_rotated_refresh" {
		t.Fatalf("stored RefreshToken = %q, want bkrt_rotated_refresh", session.RefreshToken)
	}
	if !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("stored ExpiresAt = %s, want a future timestamp", session.ExpiresAt)
	}
}
