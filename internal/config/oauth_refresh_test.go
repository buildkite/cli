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

func TestRefreshedAPITokenForOrgRefreshesStoredOAuthSession(t *testing.T) {
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

	token := conf.RefreshedAPITokenForOrg("test-org")
	if token != "bkua_refreshed_access" {
		t.Fatalf("RefreshedAPITokenForOrg() = %q, want bkua_refreshed_access", token)
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

func TestAPITokenForOrgDoesNotRefreshStoredOAuthSession(t *testing.T) {
	keyring.MockForTesting()
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

	conf := New(afero.NewMemMapFs(), nil)
	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         server.URL,
		ClientID:     "stored-client",
		AccessToken:  "bkua_current_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.APITokenForOrg("test-org"); token != "bkua_current_access" {
		t.Fatalf("APITokenForOrg() = %q, want bkua_current_access", token)
	}
	if requests != 0 {
		t.Fatalf("refresh requests = %d, want 0", requests)
	}
}

func TestAPITokenForOrgDoesNotReturnExpiredNonRefreshableToken(t *testing.T) {
	keyring.MockForTesting()

	conf := New(afero.NewMemMapFs(), nil)
	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:     oauth.SessionVersion,
		Host:        "buildkite.localhost",
		AccessToken: "bkua_expired_access",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.APITokenForOrg("test-org"); token != "" {
		t.Fatalf("APITokenForOrg() = %q, want empty token", token)
	}
}

func TestAPITokenForOrgDoesNotReturnExpiredRefreshableToken(t *testing.T) {
	keyring.MockForTesting()

	conf := New(afero.NewMemMapFs(), nil)
	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         "buildkite.localhost",
		ClientID:     "buildkite-cli",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.APITokenForOrg("test-org"); token != "" {
		t.Fatalf("APITokenForOrg() = %q, want empty token", token)
	}
}

func TestRefreshedAPITokenForOrgRefreshUsesStoredClientID(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm returned error: %v", err)
		}
		if got := r.Form.Get("client_id"); got != "stored-client" {
			t.Fatalf("client_id = %q, want stored-client", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			AccessToken:  "bkua_refreshed_access",
			RefreshToken: "bkrt_rotated_refresh",
			TokenType:    "Bearer",
			Scope:        "read_user",
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
		ClientID:     "stored-client",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.RefreshedAPITokenForOrg("test-org"); token != "bkua_refreshed_access" {
		t.Fatalf("RefreshedAPITokenForOrg() = %q, want bkua_refreshed_access", token)
	}
}

func TestRefreshedAPITokenForOrgDoesNotReturnExpiredTokenWhenRefreshFails(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			Error:     "invalid_grant",
			ErrorDesc: "refresh token is invalid",
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
		ClientID:     "stored-client",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.RefreshedAPITokenForOrg("test-org"); token != "" {
		t.Fatalf("RefreshedAPITokenForOrg() = %q, want empty token", token)
	}
}

func TestRefreshedAPITokenForOrgPropagatesRotatedSessionToSiblingOrganizations(t *testing.T) {
	keyring.MockForTesting()
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			AccessToken:  "bkua_refreshed_access",
			RefreshToken: "bkrt_rotated_refresh",
			TokenType:    "Bearer",
			Scope:        "read_user",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	conf := New(afero.NewMemMapFs(), nil)
	if err := conf.EnsureOrganization("org-a"); err != nil {
		t.Fatalf("EnsureOrganization org-a returned error: %v", err)
	}
	if err := conf.EnsureOrganization("org-b"); err != nil {
		t.Fatalf("EnsureOrganization org-b returned error: %v", err)
	}

	kr := keyring.New()
	original := &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         server.URL,
		ClientID:     "stored-client",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}
	if err := kr.SetSession("org-a", original); err != nil {
		t.Fatalf("SetSession org-a returned error: %v", err)
	}
	if err := kr.SetSession("org-b", original); err != nil {
		t.Fatalf("SetSession org-b returned error: %v", err)
	}

	if token := conf.RefreshedAPITokenForOrg("org-a"); token != "bkua_refreshed_access" {
		t.Fatalf("RefreshedAPITokenForOrg() = %q, want bkua_refreshed_access", token)
	}

	for _, org := range []string{"org-a", "org-b"} {
		session, err := kr.GetSession(org)
		if err != nil {
			t.Fatalf("GetSession(%q) returned error: %v", org, err)
		}
		if session.AccessToken != "bkua_refreshed_access" {
			t.Fatalf("stored AccessToken for %q = %q, want bkua_refreshed_access", org, session.AccessToken)
		}
		if session.RefreshToken != "bkrt_rotated_refresh" {
			t.Fatalf("stored RefreshToken for %q = %q, want bkrt_rotated_refresh", org, session.RefreshToken)
		}
	}
}

func TestRefreshedAPITokenForOrgDoesNotReturnExpiredNonRefreshableToken(t *testing.T) {
	keyring.MockForTesting()

	conf := New(afero.NewMemMapFs(), nil)
	kr := keyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:     oauth.SessionVersion,
		Host:        "buildkite.localhost",
		AccessToken: "bkua_expired_access",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	if token := conf.RefreshedAPITokenForOrg("test-org"); token != "" {
		t.Fatalf("RefreshedAPITokenForOrg() = %q, want empty token", token)
	}
}
