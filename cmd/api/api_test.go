package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

func TestBuildFullEndpoint(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		endpoint     string
		orgSlug      string
		isAnalytics  bool
		wantEndpoint string
	}{
		"endpoint with leading slash": {
			endpoint:     "/pipelines/dummy/builds/5085",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/pipelines/dummy/builds/5085",
		},
		"endpoint without leading slash": {
			endpoint:     "pipelines/dummy/builds/5085",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/pipelines/dummy/builds/5085",
		},
		"empty endpoint": {
			endpoint:     "",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/",
		},
		"root endpoint": {
			endpoint:     "/",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/",
		},
		"analytics endpoint with leading slash": {
			endpoint:     "/suites",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/analytics/organizations/test-org/suites",
		},
		"analytics endpoint without leading slash": {
			endpoint:     "suites",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/analytics/organizations/test-org/suites",
		},
		"pipeline endpoint without leading slash": {
			endpoint:     "pipelines",
			orgSlug:      "acme-inc",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/acme-inc/pipelines",
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := buildFullEndpoint(tc.endpoint, tc.orgSlug, tc.isAnalytics)

			if got != tc.wantEndpoint {
				t.Errorf("buildFullEndpoint(%q, %q, %v) = %q, want %q",
					tc.endpoint, tc.orgSlug, tc.isAnalytics, got, tc.wantEndpoint)
			}
		})
	}
}

func TestNewRESTClient_UsesFactoryRefreshAwareHTTPClient(t *testing.T) {
	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh-token","token_type":"Bearer","expires_in":3600}`))
		case "/v2/organizations/test-org/test":
			if r.Header.Get("Authorization") == "Bearer old-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
	t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))

	kr := keyring.New()
	if err := kr.Set("test-org", "old-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := kr.SetRefreshToken("test-org", "old-refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() error = %v", err)
	}

	rl := httpClient.NewRateLimitTransport(nil)
	f, err := factory.New(factory.WithTransport(rl))
	if err != nil {
		t.Fatalf("factory.New() error = %v", err)
	}

	client := newRESTClient(f)

	var response map[string]bool
	if err := client.Get(context.Background(), "/v2/organizations/test-org/test", &response); err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}
	if !response["ok"] {
		t.Fatalf("expected ok response, got %#v", response)
	}

	if got := f.Config.APITokenForOrg("test-org"); got != "new-token" {
		t.Fatalf("expected refreshed access token in keyring, got %q", got)
	}
	if got := f.Config.RefreshTokenForOrg("test-org"); got != "new-refresh-token" {
		t.Fatalf("expected rotated refresh token in keyring, got %q", got)
	}
}
