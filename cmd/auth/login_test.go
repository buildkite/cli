package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
)

func TestResolveOrganizationFromTokenUsesConfiguredBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer bkua_test_token" {
			t.Fatalf("Authorization = %q, want Bearer bkua_test_token", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]map[string]any{{"slug": "test-org"}}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	org, err := resolveOrganizationFromToken(context.Background(), server.URL, "bkua_test_token")
	if err != nil {
		t.Fatalf("resolveOrganizationFromToken returned error: %v", err)
	}
	if org == nil {
		t.Fatal("resolveOrganizationFromToken returned nil organization")
	}
	if org.Slug != "test-org" {
		t.Fatalf("Slug = %q, want test-org", org.Slug)
	}
}

func TestStoreSessionForOrganizationsStoresAllAccessibleOrgs(t *testing.T) {
	keyring.MockForTesting()

	f := &factory.Factory{
		Config: config.New(afero.NewMemMapFs(), nil),
	}
	session := &oauth.Session{
		Version:     oauth.SessionVersion,
		AccessToken: "bkua_access",
		TokenType:   "Bearer",
	}

	orgs := []buildkite.Organization{
		{Slug: "test-org"},
		{Slug: "other-org"},
		{Slug: "other-org"},
	}

	if err := storeSessionForOrganizations(f, orgs, session); err != nil {
		t.Fatalf("storeSessionForOrganizations returned error: %v", err)
	}

	kr := keyring.New()
	for _, slug := range []string{"test-org", "other-org"} {
		storedSession, err := kr.GetSession(slug)
		if err != nil {
			t.Fatalf("GetSession(%q) returned error: %v", slug, err)
		}
		if storedSession.AccessToken != "bkua_access" {
			t.Fatalf("stored access token for %q = %q, want bkua_access", slug, storedSession.AccessToken)
		}
	}

	if got := f.Config.OrganizationSlug(); got != "test-org" {
		t.Fatalf("OrganizationSlug() = %q, want test-org", got)
	}
}

func TestResolveOrganizationsFromTokenPaginates(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			w.Header().Set("Link", `<`+server.URL+`/v2/organizations?page=2>; rel="next"`)
			if err := json.NewEncoder(w).Encode([]map[string]any{{"slug": "org-one"}}); err != nil {
				t.Fatalf("Encode page 1 returned error: %v", err)
			}
		case "2":
			if err := json.NewEncoder(w).Encode([]map[string]any{{"slug": "org-two"}}); err != nil {
				t.Fatalf("Encode page 2 returned error: %v", err)
			}
		default:
			t.Fatalf("unexpected page query %q", page)
		}
	}))
	defer server.Close()

	orgs, err := resolveOrganizationsFromToken(context.Background(), server.URL, "bkua_test_token")
	if err != nil {
		t.Fatalf("resolveOrganizationsFromToken returned error: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("len(orgs) = %d, want 2", len(orgs))
	}
	if orgs[0].Slug != "org-one" || orgs[1].Slug != "org-two" {
		t.Fatalf("org slugs = [%q %q], want [org-one org-two]", orgs[0].Slug, orgs[1].Slug)
	}
}
