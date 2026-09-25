package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
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

func TestApiDataRequestBody(t *testing.T) {
	largeJSON := `{"grants":[{"name":"` + strings.Repeat("grant-value", 4000) + `"}]}`
	if len(largeJSON) <= 32*1024 {
		t.Fatal("fixture must exceed 32 KiB")
	}

	for _, tc := range []struct {
		name, data, stdin, wantBody string
	}{
		{"stdin JSON", "-", largeJSON, largeJSON},
		{"inline JSON", largeJSON, "", largeJSON},
		{"inline non-JSON", "plain text", "", `"plain text"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v2/organizations/test-org/test" {
					t.Errorf("request = %s %s, want POST /v2/organizations/test-org/test", r.Method, r.URL.Path)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				if string(body) != tc.wantBody {
					t.Errorf("request body length = %d, want %d; body matches: %v", len(body), len(tc.wantBody), string(body) == tc.wantBody)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

			stdin, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			writeResult := make(chan error, 1)
			go func() {
				_, err := io.WriteString(writer, tc.stdin)
				if closeErr := writer.Close(); err == nil {
					err = closeErr
				}
				writeResult <- err
			}()
			oldStdin := os.Stdin
			os.Stdin = stdin
			defer func() { os.Stdin = oldStdin }()

			var command struct {
				Api ApiCmd `cmd:""`
			}
			parser, err := kong.New(&command)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse([]string{"api", "/test", "--data", tc.data})
			if err != nil {
				t.Fatal(err)
			}
			if err := command.Api.Run(ctx, cli.Globals{}); err != nil {
				t.Fatal(err)
			}
			if err := <-writeResult; err != nil {
				t.Fatalf("writing stdin: %v", err)
			}
		})
	}
}

func TestApiDataStdinReadError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request sent after stdin read error")
	}))
	defer server.Close()
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = stdin
	defer func() { os.Stdin = oldStdin }()

	var command struct {
		Api ApiCmd `cmd:""`
	}
	parser, err := kong.New(&command)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := parser.Parse([]string{"api", "/test", "--data", "-"})
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Api.Run(ctx, cli.Globals{}); err == nil || !strings.Contains(err.Error(), "reading request body from stdin") {
		t.Fatalf("Run() error = %v, want stdin read error", err)
	}
}
