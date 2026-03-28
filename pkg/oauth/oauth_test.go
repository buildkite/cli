package oauth

import (
	"strings"
	"testing"
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
