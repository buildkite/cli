package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
