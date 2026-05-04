package oauth

import (
	"net/url"
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

	// Verify all scopes are present
	for _, s := range AllScopes {
		if !strings.Contains(authURL, s) {
			t.Errorf("expected scope %q in URL, got: %s", s, authURL)
		}
	}
}

func TestAuthorizationURL_IncludesOrganizationSlug(t *testing.T) {
	t.Parallel()

	flow, err := NewFlow(&Config{
		Host:        "buildkite.com",
		ClientID:    "test-client",
		CallbackURL: "http://localhost:9999/callback",
		OrgSlug:     "buildkite",
		Scopes:      "read_user",
	})
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	authURL, err := url.Parse(flow.AuthorizationURL())
	if err != nil {
		t.Fatalf("Parse AuthorizationURL: %v", err)
	}

	query := authURL.Query()
	if got := query.Get("organization"); got != "buildkite" {
		t.Fatalf("organization = %q, want %q", got, "buildkite")
	}
	if got := query.Get("organization_uuid"); got != "" {
		t.Fatalf("organization_uuid = %q, want empty", got)
	}
}

func TestAuthorizationURL_IncludesOrganizationUUID(t *testing.T) {
	t.Parallel()

	const orgUUID = "018f2f7e-7e99-7d77-b4d3-a95cb01805f4"

	flow, err := NewFlow(&Config{
		Host:        "buildkite.com",
		ClientID:    "test-client",
		CallbackURL: "http://localhost:9999/callback",
		OrgUUID:     orgUUID,
		Scopes:      "read_user",
	})
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	authURL, err := url.Parse(flow.AuthorizationURL())
	if err != nil {
		t.Fatalf("Parse AuthorizationURL: %v", err)
	}

	query := authURL.Query()
	if got := query.Get("organization_uuid"); got != orgUUID {
		t.Fatalf("organization_uuid = %q, want %q", got, orgUUID)
	}
	if got := query.Get("organization"); got != "" {
		t.Fatalf("organization = %q, want empty", got)
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
	// Should use the provided scopes, not all scopes
	if strings.Contains(authURL, "write_builds") {
		t.Errorf("expected only provided scopes, but found write_builds in URL: %s", authURL)
	}
}
