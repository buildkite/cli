package oauth

import (
	"strings"
	"testing"
	"time"
)

func TestResolveScopes(t *testing.T) {
	t.Parallel()

	readOnlyExpanded := strings.Join(ScopeGroups["read_only"], " ")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "individual scopes pass through",
			input: "read_user write_builds",
			want:  "read_user write_builds",
		},
		{
			name:  "read_only group expands",
			input: "read_only",
			want:  readOnlyExpanded,
		},
		{
			name:  "group mixed with individual scopes",
			input: "read_only write_builds",
			want:  readOnlyExpanded + " write_builds",
		},
		{
			name:  "duplicate scopes are deduplicated",
			input: "read_only read_user read_builds",
			want:  readOnlyExpanded,
		},
		{
			name:  "unknown group names pass through as literal scopes",
			input: "custom_scope",
			want:  "custom_scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveScopes(tt.input)
			if got != tt.want {
				t.Errorf("ResolveScopes(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewFlow_DefaultsToAllScopes(t *testing.T) {
	t.Parallel()

	flow, err := NewFlow(&Config{
		Host:        "buildkite.com",
		ClientID:    "test-client",
		CallbackURL: "http://localhost:9999/callback",
		Scopes:      "",
	})
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	authURL := flow.AuthorizationURL()
	if !strings.Contains(authURL, "scope=") {
		t.Fatal("expected scope parameter in URL")
	}

	for _, s := range AllScopes {
		if !strings.Contains(authURL, s) {
			t.Errorf("expected scope %q in URL, got: %s", s, authURL)
		}
	}
}

func TestAuthorizationURL_UsesProvidedScopes(t *testing.T) {
	t.Parallel()

	flow, err := NewFlow(&Config{
		Host:        "buildkite.com",
		ClientID:    "test-client",
		CallbackURL: "http://localhost:9999/callback",
		Scopes:      "read_user read_builds",
	})
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	authURL := flow.AuthorizationURL()
	if !strings.Contains(authURL, "scope=") {
		t.Fatal("expected scope parameter in URL")
	}
	if strings.Contains(authURL, "write_builds") {
		t.Errorf("expected only provided scopes, but found write_builds in URL: %s", authURL)
	}
}

func TestNewFlowUsesExplicitClientID(t *testing.T) {
	t.Setenv(EnvClientID, "env-client")
	t.Setenv(LegacyEnvClientID, "legacy-client")

	flow, err := NewFlow(&Config{
		ClientID:    "explicit-client",
		CallbackURL: "http://127.0.0.1/callback",
	})
	if err != nil {
		t.Fatalf("NewFlow returned error: %v", err)
	}

	if flow.config.ClientID != "explicit-client" {
		t.Fatalf("expected explicit client ID, got %q", flow.config.ClientID)
	}
}

func TestNewFlowUsesRuntimeClientIDOverride(t *testing.T) {
	originalDefault := DefaultClientID
	DefaultClientID = ""
	t.Cleanup(func() {
		DefaultClientID = originalDefault
	})

	t.Setenv(EnvClientID, "env-client")

	flow, err := NewFlow(&Config{
		CallbackURL: "http://127.0.0.1/callback",
	})
	if err != nil {
		t.Fatalf("NewFlow returned error: %v", err)
	}

	if flow.config.ClientID != "env-client" {
		t.Fatalf("expected runtime client ID override, got %q", flow.config.ClientID)
	}
}

func TestNewFlowUsesLegacyRuntimeClientIDFallback(t *testing.T) {
	originalDefault := DefaultClientID
	DefaultClientID = ""
	t.Cleanup(func() {
		DefaultClientID = originalDefault
	})

	t.Setenv(EnvClientID, "")
	t.Setenv(LegacyEnvClientID, "legacy-client")

	flow, err := NewFlow(&Config{
		CallbackURL: "http://127.0.0.1/callback",
	})
	if err != nil {
		t.Fatalf("NewFlow returned error: %v", err)
	}

	if flow.config.ClientID != "legacy-client" {
		t.Fatalf("expected legacy runtime client ID fallback, got %q", flow.config.ClientID)
	}
}

func TestNewFlowUsesLinkerInjectedDefaultClientID(t *testing.T) {
	originalDefault := DefaultClientID
	DefaultClientID = "linked-client"
	t.Cleanup(func() {
		DefaultClientID = originalDefault
	})

	t.Setenv(EnvClientID, "")
	t.Setenv(LegacyEnvClientID, "")

	flow, err := NewFlow(&Config{
		CallbackURL: "http://127.0.0.1/callback",
	})
	if err != nil {
		t.Fatalf("NewFlow returned error: %v", err)
	}

	if flow.config.ClientID != "linked-client" {
		t.Fatalf("expected linker-injected client ID, got %q", flow.config.ClientID)
	}
}

func TestNewFlowErrorsWhenNoClientIDIsConfigured(t *testing.T) {
	originalDefault := DefaultClientID
	DefaultClientID = ""
	t.Cleanup(func() {
		DefaultClientID = originalDefault
	})

	t.Setenv(EnvClientID, "")
	t.Setenv(LegacyEnvClientID, "")

	if _, err := NewFlow(&Config{
		CallbackURL: "http://127.0.0.1/callback",
	}); err == nil {
		t.Fatal("expected error when client ID is not configured")
	}
}

func TestTokenResponseSessionPersistsClientID(t *testing.T) {
	now := time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC)

	session := (&TokenResponse{
		AccessToken:  "bkua_access",
		RefreshToken: "bkrt_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresIn:    3600,
	}).Session("buildkite.localhost", "buildkite-cli", now)

	if session.ClientID != "buildkite-cli" {
		t.Fatalf("ClientID = %q, want buildkite-cli", session.ClientID)
	}
	if got, want := session.ExpiresAt, now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", got, want)
	}
}

func TestSessionUpdateKeepsSessionRefreshableWhenExpiresInIsOmitted(t *testing.T) {
	now := time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC)
	originalExpiry := now.Add(-2 * time.Hour)

	updated := (&Session{
		Version:      SessionVersion,
		Host:         "buildkite.localhost",
		ClientID:     "buildkite-cli",
		AccessToken:  "bkua_old_access",
		RefreshToken: "bkrt_old_refresh",
		TokenType:    "Bearer",
		Scope:        "read_user",
		ExpiresAt:    originalExpiry,
	}).Update(&TokenResponse{
		AccessToken: "bkua_new_access",
	}, now)

	if updated.ClientID != "buildkite-cli" {
		t.Fatalf("ClientID = %q, want buildkite-cli", updated.ClientID)
	}
	if updated.RefreshToken != "bkrt_old_refresh" {
		t.Fatalf("RefreshToken = %q, want bkrt_old_refresh", updated.RefreshToken)
	}
	if updated.Scope != "read_user" {
		t.Fatalf("Scope = %q, want read_user", updated.Scope)
	}
	if updated.TokenType != "Bearer" {
		t.Fatalf("TokenType = %q, want Bearer", updated.TokenType)
	}
	if !updated.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt = %s, want zero value when expires_in is omitted", updated.ExpiresAt)
	}
	if !updated.CanRefresh() {
		t.Fatal("expected updated session to remain refreshable")
	}
	if !updated.NeedsRefresh(now) {
		t.Fatal("expected updated session to refresh again when expiry is unknown")
	}
}

func TestSessionWithoutExpiryStillRefreshes(t *testing.T) {
	session := &Session{
		RefreshToken: "bkrt_refresh",
	}

	if !session.CanRefresh() {
		t.Fatal("expected session with refresh token to be refreshable")
	}
	if !session.NeedsRefresh(time.Now()) {
		t.Fatal("expected session without expiry to refresh before use")
	}
}
