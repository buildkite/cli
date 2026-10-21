package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/keyring"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/spf13/afero"
)

func TestFactoryDoesNotBorrowCredentials(t *testing.T) {
	for _, targetRefresh := range []string{"", "target-refresh-only"} {
		name := "no target credentials"
		if targetRefresh != "" {
			name = "target refresh token only"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "selected")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			kr := keyring.New()
			if err := kr.Set("selected", "selected-access"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("selected", "selected-refresh"); err != nil {
				t.Fatal(err)
			}
			if targetRefresh != "" {
				if err := kr.SetRefreshToken("target", targetRefresh); err != nil {
					t.Fatal(err)
				}
			}
			requests, refreshes := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					refreshes++
				} else {
					requests++
					if r.URL.Path != "/v2/organizations/target/pipelines/widgets/builds" {
						t.Errorf("unexpected request path: %s", r.URL.Path)
					}
					// The SDK emits an empty Bearer header when no token is supplied.
					if auth := r.Header.Get("Authorization"); auth != "" && auth != "Bearer" {
						t.Error("factory injected credentials despite missing target access token")
					}
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()
			transport := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = transport }()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
			f, err := New(WithOrgOverride("target"))
			if err != nil {
				t.Fatal(err)
			}
			// Bypass command validation to test the factory's own credential policy.
			_, response, err := f.RestAPIClient.Builds.ListByPipeline(context.Background(), "target", "widgets", nil)
			if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized || requests != 1 || refreshes != 0 {
				t.Fatalf("expected one unauthenticated request without refresh: err=%v requests=%d refreshes=%d", err, requests, refreshes)
			}
		})
	}
}

func TestRefreshOnlyForAuthenticatedAPIOrigins(t *testing.T) {
	for _, test := range []struct {
		name            string
		start           string
		redirect        string
		wantRefresh     bool
		omitInitialAuth bool
		restEndpoint    string
	}{
		{"external storage", "https://api.example/proxy/log", "https://storage.example/log?signature=synthetic", false, false, ""},
		{"API subdomain", "https://api.example/proxy/log", "https://storage.api.example/log", false, false, ""},
		{"different port", "https://api.example/proxy/log", "https://api.example:8443/log", false, false, ""},
		{"different scheme", "https://api.example/proxy/log", "http://api.example/log", false, false, ""},
		{"REST API", "https://api.example/proxy/log", "", true, false, ""},
		{"GraphQL API", "https://graphql.example/custom/query", "", true, false, ""},
		{"same origin redirect", "https://api.example/proxy/log", "https://api.example/proxy/download", true, false, ""},
		{"unauthenticated API redirect", "https://api.example/proxy/log", "https://graphql.example/custom/download", false, true, ""},
		{"explicit HTTPS default port", "https://api.example/proxy/log", "https://api.example:443/proxy/download", true, false, ""},
		{"omitted HTTPS default port", "https://api.example:443/proxy/log", "https://api.example/proxy/download", true, false, "https://api.example:443/proxy/"},
		{"explicit HTTP default port", "http://api.example/proxy/log", "http://api.example:80/proxy/download", true, false, "http://api.example/proxy/"},
		{"omitted HTTP default port", "http://api.example:80/proxy/log", "http://api.example/proxy/download", true, false, "http://api.example:80/proxy/"},
		{"padded HTTPS default port", "https://api.example/proxy/log", "https://api.example:0443/proxy/download", true, false, ""},
		{"omitted padded HTTPS port", "https://api.example:0443/proxy/log", "https://api.example/proxy/download", true, false, "https://api.example:0443/proxy/"},
		{"explicit to padded HTTPS port", "https://api.example:443/proxy/log", "https://api.example:0443/proxy/download", true, false, "https://api.example:443/proxy/"},
		{"padded to explicit HTTPS port", "https://api.example:0443/proxy/log", "https://api.example:443/proxy/download", true, false, "https://api.example:0443/proxy/"},
		{"padded HTTP default port", "http://api.example/proxy/log", "http://api.example:0080/proxy/download", true, false, "http://api.example/proxy/"},
		{"omitted padded HTTP port", "http://api.example:0080/proxy/log", "http://api.example/proxy/download", true, false, "http://api.example:0080/proxy/"},
		{"explicit to padded HTTP port", "http://api.example:80/proxy/log", "http://api.example:0080/proxy/download", true, false, "http://api.example:80/proxy/"},
		{"padded to explicit HTTP port", "http://api.example:0080/proxy/log", "http://api.example:80/proxy/download", true, false, "http://api.example:0080/proxy/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", "https://api.example/proxy/")
			if test.restEndpoint != "" {
				t.Setenv("BUILDKITE_REST_API_ENDPOINT", test.restEndpoint)
			}
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", "https://graphql.example/custom/query")
			t.Setenv("BUILDKITE_HOST", "oauth.example")
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			kr := keyring.New()
			if err := kr.Set("test", "synthetic-token"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("test", "synthetic-refresh"); err != nil {
				t.Fatal(err)
			}

			requests, refreshes := 0, 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				resp := &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("unauthorized")),
					Request:    req,
				}
				if req.URL.String() == "https://oauth.example/oauth/token" {
					refreshes++
					resp.StatusCode = http.StatusOK
					resp.Body = io.NopCloser(strings.NewReader(`{"access_token":"rotated-token","refresh_token":"rotated-refresh"}`))
					return resp, nil
				}
				requests++
				if test.redirect != "" && req.URL.String() == test.start {
					resp.StatusCode = http.StatusFound
					resp.Header.Set("Location", test.redirect)
					return resp, nil
				}
				if !test.wantRefresh && req.Header.Get("Authorization") == "Bearer rotated-token" {
					t.Error("refreshed API credential reached non-API origin")
				}
				if test.name == "external storage" && req.Header.Get("Authorization") != "" {
					t.Error("synthetic Buildkite credential reached redirected storage host after 401")
				}
				if test.wantRefresh && req.Header.Get("Authorization") == "Bearer rotated-token" {
					resp.StatusCode = http.StatusOK
				}
				return resp, nil
			})
			// Keep OAuth exchanges synthetic too. No request can reach the network.
			original := http.DefaultTransport
			http.DefaultTransport = transport
			defer func() { http.DefaultTransport = original }()
			f, err := New(WithTransport(transport))
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodGet, test.start, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Match the REST SDK's initial header, including stdlib forwarding
			// to subdomains and same-host redirects across ports or schemes.
			if !test.omitInitialAuth {
				req.Header.Set("Authorization", "Bearer synthetic-token")
			}
			resp, err := f.HTTPClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			wantStatus, wantRequests, wantRefreshes := http.StatusUnauthorized, 1, 0
			if test.redirect != "" {
				wantRequests++
			}
			if test.wantRefresh {
				wantStatus = http.StatusOK
				wantRequests++
				wantRefreshes = 1
			}
			if resp.StatusCode != wantStatus || requests != wantRequests || refreshes != wantRefreshes {
				t.Fatalf("status=%d requests=%d refreshes=%d; want %d, %d, %d", resp.StatusCode, requests, refreshes, wantStatus, wantRequests, wantRefreshes)
			}
		})
	}
}

func TestRedirectsDoNotRestoreStrippedIdentity(t *testing.T) {
	for _, test := range []struct {
		name             string
		environmentToken bool
		withBasic        bool
		returnStatus     int
		finalStatus      int
		directReturn     bool
	}{
		{"Basic then success without refresh transport", true, true, http.StatusFound, http.StatusOK, false},
		{"Basic 401 with refresh transport", false, true, http.StatusUnauthorized, 0, false},
		{"Basic then unauthenticated 401", false, true, http.StatusFound, http.StatusUnauthorized, false},
		{"unauthenticated return to API", false, false, http.StatusFound, http.StatusUnauthorized, false},
		{"direct Basic then success", false, true, http.StatusFound, http.StatusOK, true},
		{"direct Basic 401", false, true, http.StatusUnauthorized, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			if test.environmentToken {
				t.Setenv("BUILDKITE_API_TOKEN", "synthetic-token")
			}
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", "https://api.example/")
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", "https://graphql.example/query")
			t.Setenv("BUILDKITE_HOST", "oauth.example")
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			kr := keyring.New()
			if err := kr.Set("test", "synthetic-token"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("test", "synthetic-refresh"); err != nil {
				t.Fatal(err)
			}
			requests := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				resp := &http.Response{StatusCode: http.StatusFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}
				wantAuth := ""
				switch req.URL.String() {
				case "https://api.example/start":
					wantAuth = "Bearer synthetic-token"
					resp.Header.Set("Location", "https://storage.example/blob")
					if test.directReturn {
						resp.Header.Set("Location", "https://user:pass@api.example/return")
					}
				case "https://storage.example/blob":
					location := "https://api.example/return"
					if test.withBasic {
						location = "https://user:pass@api.example/return"
					}
					resp.Header.Set("Location", location)
				case "https://user:pass@api.example/return", "https://api.example/return":
					if test.withBasic {
						wantAuth = "Basic dXNlcjpwYXNz"
					}
					resp.StatusCode = test.returnStatus
					resp.Header.Set("Location", "https://api.example/final")
				case "https://api.example/final":
					resp.StatusCode = test.finalStatus
				default:
					t.Errorf("unexpected request to %s", req.URL.Redacted())
					resp.StatusCode = http.StatusUnauthorized
				}
				if got := req.Header.Get("Authorization"); got != wantAuth {
					t.Errorf("unexpected identity at %s: Authorization=%q, want %q", req.URL.Redacted(), got, wantAuth)
				}
				return resp, nil
			})
			original := http.DefaultTransport
			http.DefaultTransport = transport
			defer func() { http.DefaultTransport = original }()
			f, err := New(WithTransport(transport))
			if err != nil {
				t.Fatal(err)
			}
			req, _ := http.NewRequest(http.MethodGet, "https://api.example/start", nil)
			if !test.directReturn {
				req.Header.Set("Authorization", "Bearer synthetic-token")
			}
			resp, err := f.HTTPClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			wantStatus, wantRequests := test.finalStatus, 4
			if test.returnStatus == http.StatusUnauthorized {
				wantStatus, wantRequests = http.StatusUnauthorized, 3
			}
			if test.directReturn {
				wantRequests--
			}
			if resp.StatusCode != wantStatus || requests != wantRequests {
				t.Errorf("status=%d requests=%d; want %d, %d", resp.StatusCode, requests, wantStatus, wantRequests)
			}
		})
	}
}

func TestRedirectHostnameIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		start    string
		target   string
		wantAuth bool
	}{
		{"distinct Greek hosts", "https://σ.example/start", "https://ς.example/final", false},
		{"Unicode to IDNA", "https://σ.example/start", "https://xn--4xa.example/final", true},
		{"IDNA to Unicode", "https://xn--4xa.example/start", "https://σ.example/final", true},
		{"ASCII case", "https://API.example/start", "https://api.example/final", true},
		{"IPv6 spelling", "https://[::1]/start", "https://[0:0:0:0:0:0:0:1]/final", true},
		{"distinct IPv6 zones", "https://[fe80::1%25en0]/start", "https://[fe80::1%25EN0]/final", false},
		{"ASCII to fullwidth", "https://a.example/start", "https://ａ.example/final", false},
		{"fullwidth to ASCII", "https://ａ.example/start", "https://a.example/final", false},
		{"fullwidth to wire IDNA", "https://ａ.example/start", "https://xn--mi7c.example/final", false},
		{"wire IDNA to fullwidth", "https://xn--mi7c.example/start", "https://ａ.example/final", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "synthetic-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", test.start)
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", "https://graphql.example/query")
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			requests := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}
				wantAuth := "Bearer synthetic-token"
				if req.URL.Path == "/start" {
					resp.StatusCode = http.StatusFound
					resp.Header.Set("Location", test.target)
				} else if !test.wantAuth {
					wantAuth = ""
				}
				if got := req.Header.Get("Authorization"); got != wantAuth {
					t.Errorf("Authorization at %s=%q, want %q", req.URL, got, wantAuth)
				}
				return resp, nil
			})
			f, err := New(WithTransport(transport))
			if err != nil {
				t.Fatal(err)
			}
			// No initial header: auth is transport-injected, as with GraphQL.
			resp, err := f.HTTPClient.Get(test.start)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if requests != 2 || resp.StatusCode != http.StatusOK {
				t.Fatalf("expected redirect and final 200, got requests=%d status=%d", requests, resp.StatusCode)
			}
		})
	}
}

func TestRedirectDoesNotAuthenticateDifferentProxyAuthority(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "synthetic-token")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test")
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", "http://a.example/")
	keyring.MockForTesting()
	defer keyring.ResetForTesting()
	var requests atomic.Int32
	// This loopback proxy answers both requests itself; no DNS or upstream
	// connection occurs. Verify the actual absolute request-target and Host.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/start" {
			if r.Header.Get("Authorization") != "Bearer synthetic-token" {
				t.Error("expected authenticated initial API request")
			}
			http.Redirect(w, r, "http://ａ.example/final", http.StatusFound)
			return
		}
		if r.URL.Host != "xn--mi7c.example" || r.Host != "xn--mi7c.example" {
			t.Errorf("unexpected wire authority: target=%s Host=%s", r.URL, r.Host)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("API credential reached a different proxy authority: %q", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	f, err := New(WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := f.HTTPClient.Get("http://a.example/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if requests.Load() != 2 || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected redirect and final 200, got requests=%d status=%d", requests.Load(), resp.StatusCode)
	}
}

func TestGraphQLSameOriginRedirect(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		expired bool
	}{
		{"307 valid token", http.StatusTemporaryRedirect, false},
		{"307 expired token", http.StatusTemporaryRedirect, true},
		{"308 valid token", http.StatusPermanentRedirect, false},
		{"308 expired token", http.StatusPermanentRedirect, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			kr := keyring.New()
			if err := kr.Set("test", "synthetic-token"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("test", "synthetic-refresh"); err != nil {
				t.Fatal(err)
			}
			const query = "query RedirectProbe { ok }"
			var refreshes, downloads atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					refreshes.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"rotated-token","refresh_token":"rotated-refresh"}`)
					return
				}
				var body struct{ Query string }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Query != query || r.Method != http.MethodPost {
					t.Errorf("GraphQL POST body not preserved: method=%s query=%q err=%v", r.Method, body.Query, err)
				}
				if r.URL.Path == "/graphql" {
					http.Redirect(w, r, "/redirected", test.status)
					return
				}
				downloads.Add(1)
				wantToken := "synthetic-token"
				if test.expired {
					wantToken = "rotated-token"
				}
				if r.Header.Get("Authorization") != "Bearer "+wantToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
			}))
			defer server.Close()
			original := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = original }()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL+"/rest/")
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", server.URL+"/graphql")
			t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
			f, err := New()
			if err != nil {
				t.Fatal(err)
			}
			var data struct{ OK bool }
			err = f.GraphQLClient.MakeRequest(context.Background(), &graphql.Request{Query: query}, &graphql.Response{Data: &data})
			if err != nil || !data.OK {
				t.Fatalf("expected successful GraphQL response, got data=%+v err=%v", data, err)
			}
			wantRefreshes, wantDownloads := int32(0), int32(1)
			if test.expired {
				wantRefreshes, wantDownloads = 1, 2
			}
			if refreshes.Load() != wantRefreshes || downloads.Load() != wantDownloads {
				t.Errorf("refreshes=%d redirected requests=%d; want %d, %d", refreshes.Load(), downloads.Load(), wantRefreshes, wantDownloads)
			}
		})
	}
}

func TestOrganizationCredentialRefresh(t *testing.T) {
	for _, environmentToken := range []bool{false, true} {
		name := "target stored credentials refresh"
		if environmentToken {
			name = "environment credentials never refresh"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "selected")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			defer keyring.ResetForTesting()
			kr := keyring.New()
			for _, org := range []string{"selected", "target"} {
				if err := kr.Set(org, org+"-access"); err != nil {
					t.Fatal(err)
				}
				if err := kr.SetRefreshToken(org, org+"-refresh"); err != nil {
					t.Fatal(err)
				}
			}
			if environmentToken {
				t.Setenv("BUILDKITE_API_TOKEN", "explicit-access")
			}
			requests, refreshes := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/oauth/token" {
					refreshes++
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("refresh_token") != "target-refresh" {
						t.Error("refresh used the wrong organization's token")
					}
					_, _ = io.WriteString(w, `{"access_token":"rotated-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`)
					return
				}
				requests++
				if r.URL.Path != "/v2/organizations/target/pipelines/widgets/builds" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				want := "target-access"
				if environmentToken {
					want = "explicit-access"
				} else if requests > 1 {
					want = "rotated-access"
				}
				if r.Header.Get("Authorization") != "Bearer "+want {
					t.Error("request used unexpected credentials")
				}
				if requests == 1 {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, `[]`)
			}))
			defer server.Close()
			transport := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = transport }()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
			f, err := New(WithOrgOverride("target"))
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = f.RestAPIClient.Builds.ListByPipeline(context.Background(), "target", "widgets", nil)
			if environmentToken {
				if err == nil || requests != 1 || refreshes != 0 {
					t.Fatalf("expected original auth failure without refresh: err=%v requests=%d refreshes=%d", err, requests, refreshes)
				}
			} else if err != nil || requests != 2 || refreshes != 1 {
				t.Fatalf("expected target refresh and retry: err=%v requests=%d refreshes=%d", err, requests, refreshes)
			}
			for _, org := range []string{"selected", "target"} {
				wantAccess, wantRefresh := org+"-access", org+"-refresh"
				if org == "target" && !environmentToken {
					wantAccess, wantRefresh = "rotated-access", "rotated-refresh"
				}
				access, _ := kr.Get(org)
				refresh, _ := kr.GetRefreshToken(org)
				if access != wantAccess || refresh != wantRefresh {
					t.Errorf("unexpected credential changes for %s", org)
				}
			}
		})
	}
}

// withCredentialStoreEnv sets BUILDKITE_CREDENTIAL_STORE for the test (or
// unsets it when value is empty) and restores prior state via t.Cleanup.
func withCredentialStoreEnv(t *testing.T, value string) {
	t.Helper()
	original, had := os.LookupEnv(keyring.CredentialStoreEnv)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(keyring.CredentialStoreEnv, original)
			return
		}
		_ = os.Unsetenv(keyring.CredentialStoreEnv)
	})
	if value == "" {
		_ = os.Unsetenv(keyring.CredentialStoreEnv)
		return
	}
	_ = os.Setenv(keyring.CredentialStoreEnv, value)
}

func TestRedactHeaders(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		value    string
		expected string
	}{
		{
			name:     "Bearer token",
			header:   "Authorization",
			value:    "Bearer bkua_1234567890abcdef",
			expected: "Bearer [REDACTED]",
		},
		{
			name:     "Basic auth",
			header:   "Authorization",
			value:    "Basic dXNlcjpwYXNz",
			expected: "Basic [REDACTED]",
		},
		{
			name:     "Token without type",
			header:   "Authorization",
			value:    "sometoken123",
			expected: "[REDACTED]",
		},
		{
			name:     "Non-sensitive header unchanged",
			header:   "Content-Type",
			value:    "application/json",
			expected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set(tt.header, tt.value)

			redactHeaders(headers)

			got := headers.Get(tt.header)
			if got != tt.expected {
				t.Errorf("redactHeaders() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRedactHeadersMultipleValues(t *testing.T) {
	headers := http.Header{}
	headers.Add("Authorization", "Bearer token1")
	headers.Add("Authorization", "Bearer token2")

	redactHeaders(headers)

	values := headers.Values("Authorization")
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}

	for _, v := range values {
		if v != "Bearer [REDACTED]" {
			t.Errorf("expected 'Bearer [REDACTED]', got %q", v)
		}
	}
}

func TestRedactBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "form-encoded fields",
			input: `access_token=access-secret&refresh_token=refresh-secret&code=code-secret&code_verifier=verifier-secret&state=visible`,
			want:  `access_token=[REDACTED]&refresh_token=[REDACTED]&code=[REDACTED]&code_verifier=[REDACTED]&state=visible`,
		},
		{
			name:  "existing JSON fields",
			input: `{"access_token":"access-secret","refresh_token": "refresh-secret","code":"code-secret","state":"visible"}`,
			want:  `{"access_token":"[REDACTED]","refresh_token": "[REDACTED]","code":"[REDACTED]","state":"visible"}`,
		},
		{
			name:  "VNC session response",
			input: `{"endpoint":"wss://vnc.example.test/session","access_token":"namespace-secret","vnc":{"username":"vnc-user","password": "vnc-secret"}}`,
			want:  `{"endpoint":"wss://vnc.example.test/session","access_token":"[REDACTED]","vnc":{"username":"vnc-user","password": "[REDACTED]"}}`,
		},
		{
			name:  "SSH session response",
			input: `{"endpoint":"tcp://ssh.example.test:22","access_token":"namespace-secret","transport":"tcp","ssh":{"username":"root","private_key": "private-key-secret","host_keys":["ssh-ed25519 public-host-key"]}}`,
			want:  `{"endpoint":"tcp://ssh.example.test:22","access_token":"[REDACTED]","transport":"tcp","ssh":{"username":"root","private_key": "[REDACTED]","host_keys":["ssh-ed25519 public-host-key"]}}`,
		},
		{
			name:  "truncated JSON string",
			input: `{"access_token":"truncated-secret`,
			want:  `{"access_token":"[REDACTED]`,
		},
		{
			name:  "JSON whitespace and escaped values",
			input: `{"password" : "prefix\"secret-suffix", "private_key" : "key\\path\nsecret", "value":"visible"}`,
			want:  `{"password" : "[REDACTED]", "private_key" : "[REDACTED]", "value":"visible"}`,
		},
		{
			name:  "truncated JSON escaped quote and backslash",
			input: `{"password":"prefix\"secret-suffix\`,
			want:  `{"password":"[REDACTED]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := redactBody(test.input); got != test.want {
				t.Errorf("redactBody() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDebugArtifactDownloadsStreamWithoutLoggingBodies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "synthetic-token")
	keyring.MockForTesting()
	defer keyring.ResetForTesting()

	for _, test := range []struct {
		name        string
		contentType string
		redirect    bool
	}{
		{"direct binary", "application/octet-stream", false},
		{"redirected text", "text/plain", true},
		{"redirected JSON", "application/json", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := strings.Repeat("artifact-payload-not-for-debug\x00\xff", 32768)
			dest := filepath.Join(t.TempDir(), "artifact")
			streamed := make(chan bool, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					w.Header().Set("Location", "/payload")
					w.WriteHeader(http.StatusFound)
					_, _ = io.WriteString(w, "redirect-body-not-for-debug")
					return
				}
				if r.URL.Path == "/api" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"message":"ordinary-api-diagnostic","access_token":"response-secret"}`)
					return
				}
				w.Header().Set("Content-Type", test.contentType)
				w.Header().Set("X-Artifact-Metadata", "retained")
				_, _ = io.WriteString(w, payload[:64<<10])
				w.(http.Flusher).Flush()
				// Hold the remaining bytes until the destination gets data. Full-body
				// buffering instead leaves the file empty until this deadline expires.
				wroteEarly := false
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
						wroteEarly = true
						break
					}
					time.Sleep(time.Millisecond)
				}
				streamed <- wroteEarly
				_, _ = io.WriteString(w, payload[64<<10:])
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			f, err := New(WithDebug(true))
			if err != nil {
				t.Fatal(err)
			}
			log, err := os.CreateTemp(t.TempDir(), "debug.log")
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			original := os.Stderr
			os.Stderr = log
			defer func() { os.Stderr = original }()

			path := "/payload"
			if test.redirect {
				path = "/redirect"
			}
			if err := artifact.DownloadToFile(context.Background(), f.RestAPIClient, server.URL+path, dest); err != nil {
				t.Fatal(err)
			}
			if !<-streamed {
				t.Error("download did not write to the destination before the complete payload arrived")
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != payload {
				t.Fatalf("downloaded bytes changed: read error = %v", err)
			}

			resp, err := f.HTTPClient.Get(server.URL + "/api")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || !strings.Contains(string(body), "response-secret") {
				t.Fatalf("ordinary API response changed: body = %q, error = %v", body, err)
			}
			os.Stderr = original
			output, err := os.ReadFile(log.Name())
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"artifact-payload-not-for-debug", "redirect-body-not-for-debug", "response-secret"} {
				if bytes.Contains(output, []byte(marker)) {
					t.Errorf("debug output contains %q", marker)
				}
			}
			for _, marker := range []string{"200 OK", "X-Artifact-Metadata: retained", test.contentType, "ordinary-api-diagnostic", "[REDACTED]"} {
				if !bytes.Contains(output, []byte(marker)) {
					t.Errorf("debug output is missing %q", marker)
				}
			}
			if test.redirect && !bytes.Contains(output, []byte("302 Found")) {
				t.Error("debug output is missing redirect metadata")
			}
			if len(output) > 16<<10 {
				t.Errorf("debug output grew with the payload: %d bytes", len(output))
			}
		})
	}
}

func TestDebugTransportRedactsCredentialsWithoutChangingTraffic(t *testing.T) {
	requestBody := `{"access_token":"synthetic-request-token","password" : "synthetic-request-prefix\"synthetic-request-suffix","value":"visible-request"}`
	responseBody := `{"private_key" : "synthetic-key\\path\nsynthetic-key-suffix","password":"synthetic-response-prefix\"synthetic-response-suffix","value":"visible-response"}`
	requestHeaders := http.Header{
		"Authorization":       {"Bearer synthetic-request-auth"},
		"Proxy-Authorization": {"Basic synthetic-proxy-auth"},
		"Cookie":              {"session=synthetic-request-cookie; other=synthetic-other-cookie"},
		"X-Api-Key":           {"synthetic-request-api-key"},
	}
	responseHeaders := http.Header{
		"Authorization": {"Bearer synthetic-response-auth"},
		"Set-Cookie":    {"session=synthetic-response-cookie; Path=/", "other=synthetic-response-cookie-two"},
		"X-Api-Key":     {"synthetic-response-api-key"},
	}
	path := "/probe?access_token=synthetic-query-token&%61ccess_token=synthetic-encoded-token&api_key=synthetic-query-api-key&ordinary=synthetic-query-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != path {
			t.Errorf("outgoing URL changed: %s", r.URL.RequestURI())
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != requestBody {
			t.Errorf("outgoing body changed: body=%s err=%v", body, err)
		}
		for key, values := range requestHeaders {
			if got := r.Header.Values(key); strings.Join(got, "\n") != strings.Join(values, "\n") {
				t.Errorf("outgoing %s changed: %v", key, got)
			}
		}
		if r.Trailer.Get("X-Api-Key") != "synthetic-request-trailer" {
			t.Error("outgoing request trailer changed")
		}
		for key, values := range responseHeaders {
			w.Header()[key] = append([]string(nil), values...)
		}
		w.Header().Set("Trailer", "X-Api-Key")
		_, _ = io.WriteString(w, responseBody)
		w.Header().Set("X-Api-Key", "synthetic-response-trailer")
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+path+"#synthetic-fragment", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	req.URL.User = url.UserPassword("synthetic-url-user", "synthetic-url-password")
	originalURL := req.URL.String()
	req.Header = requestHeaders.Clone()
	req.ContentLength = -1
	req.Trailer = http.Header{"X-Api-Key": {"synthetic-request-trailer"}}

	stderr := captureStderr(t, func() {
		client := &http.Client{Transport: &debugTransport{transport: http.DefaultTransport}}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || string(body) != responseBody {
			t.Errorf("caller response body changed: body=%s err=%v", body, err)
		}
		for key, values := range responseHeaders {
			if got := resp.Header.Values(key); strings.Join(got, "\n") != strings.Join(values, "\n") {
				t.Errorf("caller response %s changed: %v", key, got)
			}
		}
		if resp.Trailer.Get("X-Api-Key") != "synthetic-response-trailer" {
			t.Error("caller response trailer changed")
		}
	})
	if req.URL.String() != originalURL {
		t.Error("original request URL mutated")
	}
	if strings.Contains(stderr, "synthetic-") {
		t.Errorf("stderr leaked synthetic credentials:\n%s", stderr)
	}
	for _, visible := range []string{"DEBUG request uri=", "DEBUG response uri=", "/probe", "visible-request", "visible-response", "[REDACTED]"} {
		if !strings.Contains(stderr, visible) {
			t.Errorf("stderr missing %q:\n%s", visible, stderr)
		}
	}
}

func TestDebugTransportPreservesRequestBody(t *testing.T) {
	expectedBody := `{"name":"test-pipeline","cluster_id":"","repository":"git@github.com:test/repo.git"}`

	// Create a test server that checks the request body.
	// Note: the handler runs in a separate goroutine, so we capture errors
	// in a variable rather than calling t.Fatalf (which would hang the test).
	var receivedBody string
	var handlerErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErr = err
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// Use the debug transport
	dt := &debugTransport{
		transport: http.DefaultTransport,
	}

	req, err := http.NewRequest("POST", server.URL, strings.NewReader(expectedBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if handlerErr != nil {
		t.Fatalf("handler failed to read request body: %v", handlerErr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if receivedBody != expectedBody {
		t.Errorf("request body was not preserved through debug transport\ngot:  %q\nwant: %q", receivedBody, expectedBody)
	}
}

func TestDebugTransportHandlesNilBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dt := &debugTransport{
		transport: http.DefaultTransport,
	}

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestDebugTransportOmitsClusterSecretRequestBodies(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		body          string
		secretMarkers []string
		wantVisible   string
	}{
		{
			name:          "create secret",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets",
			body:          "{\n  \"key\": \"API_KEY\",\n  \"value\" : \"secret-prefix-\\\"quoted\\\"-\\\\path\\nsecret-suffix\"\n}",
			secretMarkers: []string{"API_KEY", "secret-prefix", "quoted", "secret-suffix"},
		},
		{
			name:          "update secret value",
			method:        http.MethodPut,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets/secret-1/value",
			body:          `{ "value" : "replacement-secret" }`,
			secretMarkers: []string{"replacement-secret"},
		},
		{
			name:          "query string after secret endpoint still omits body",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets?somequerystring=something",
			body:          `{"key":"API_KEY","value":"query-string-secret"}`,
			secretMarkers: []string{"API_KEY", "query-string-secret"},
		},
		{
			name:          "malformed cluster ID still omits body",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1//secrets",
			body:          `{"key":"API_KEY","value":"malformed-id-secret"}`,
			secretMarkers: []string{"API_KEY", "malformed-id-secret"},
		},
		{
			name:          "query delimiter in cluster ID still omits body",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1?/secrets",
			body:          `{"key":"API_KEY","value":"query-delimiter-secret"}`,
			secretMarkers: []string{"API_KEY", "query-delimiter-secret"},
		},
		{
			name:          "fragment delimiter in secret ID still omits body",
			method:        http.MethodPut,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets/secret-1#/value",
			body:          `{"value":"fragment-delimiter-secret"}`,
			secretMarkers: []string{"fragment-delimiter-secret"},
		},
		{
			name:          "malformed secret request fails closed",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets",
			body:          `{"key":"API_KEY","value":"truncated-secret`,
			secretMarkers: []string{"API_KEY", "truncated-secret"},
		},
		{
			name:        "unrelated value remains visible",
			method:      http.MethodPost,
			path:        "/v2/organizations/test/pipelines",
			body:        `{"value": "useful-debug-value"}`,
			wantVisible: `"value": "useful-debug-value"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var forwardedBody string
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read forwarded request body: %v", err)
				}
				forwardedBody = string(body)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Request:    req,
				}, nil
			})

			req, err := http.NewRequest(test.method, "https://api.buildkite.com"+test.path, strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			stderr := captureStderr(t, func() {
				resp, err := (&debugTransport{transport: transport}).RoundTrip(req)
				if err != nil {
					t.Fatalf("RoundTrip: %v", err)
				}
				resp.Body.Close()
			})

			if forwardedBody != test.body {
				t.Errorf("forwarded body = %q, want %q", forwardedBody, test.body)
			}
			for _, marker := range test.secretMarkers {
				if strings.Contains(stderr, marker) {
					t.Errorf("stderr contains secret marker %q:\n%s", marker, stderr)
				}
			}
			if len(test.secretMarkers) > 0 && !strings.Contains(stderr, omittedRequestBody) {
				t.Errorf("stderr does not say the request body was omitted:\n%s", stderr)
			}
			if test.wantVisible != "" && !strings.Contains(stderr, test.wantVisible) {
				t.Errorf("stderr does not contain %q:\n%s", test.wantVisible, stderr)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = write
	t.Cleanup(func() { os.Stderr = original })

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	os.Stderr = original

	var output bytes.Buffer
	if _, err := io.Copy(&output, read); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return output.String()
}

func TestBuildUserAgent(t *testing.T) {
	t.Run("default user agent has no preflight suffix", func(t *testing.T) {
		got := buildUserAgent("")
		if !strings.Contains(got, buildkite.DefaultUserAgent) {
			t.Fatalf("expected default user agent %q in %q", buildkite.DefaultUserAgent, got)
		}
		if strings.Contains(got, "buildkite-cli-preflight/") {
			t.Fatalf("expected no preflight suffix in %q", got)
		}
	})

	t.Run("preflight suffix is appended when requested", func(t *testing.T) {
		got := buildUserAgent("buildkite-cli-preflight/3.x")
		if !strings.Contains(got, buildkite.DefaultUserAgent) {
			t.Fatalf("expected default user agent %q in %q", buildkite.DefaultUserAgent, got)
		}
		if !strings.Contains(got, "buildkite-cli-preflight/3.x") {
			t.Fatalf("expected preflight suffix in %q", got)
		}
	})
}

func TestApplyCredentialStoreFromConfig(t *testing.T) {
	t.Run("exports configured store when env unset", func(t *testing.T) {
		withCredentialStoreEnv(t, "")

		conf := config.New(afero.NewMemMapFs(), nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		ApplyCredentialStoreFromConfig(conf)

		if got := os.Getenv(keyring.CredentialStoreEnv); got != keyring.StoreSHM {
			t.Errorf("env = %q, want %q", got, keyring.StoreSHM)
		}
	})

	t.Run("env value is preserved", func(t *testing.T) {
		withCredentialStoreEnv(t, keyring.StoreKeyring)

		conf := config.New(afero.NewMemMapFs(), nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		ApplyCredentialStoreFromConfig(conf)

		if got := os.Getenv(keyring.CredentialStoreEnv); got != keyring.StoreKeyring {
			t.Errorf("env = %q, want %q (env should win)", got, keyring.StoreKeyring)
		}
	})

	t.Run("auto in config does not export env", func(t *testing.T) {
		withCredentialStoreEnv(t, "")

		conf := config.New(afero.NewMemMapFs(), nil)
		if err := conf.SetCredentialStore(keyring.StoreAuto); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		ApplyCredentialStoreFromConfig(conf)

		if got, set := os.LookupEnv(keyring.CredentialStoreEnv); set {
			t.Errorf("env should remain unset for auto, got %q", got)
		}
	})

	t.Run("missing config does not export env", func(t *testing.T) {
		withCredentialStoreEnv(t, "")

		conf := config.New(afero.NewMemMapFs(), nil)

		ApplyCredentialStoreFromConfig(conf)

		if got, set := os.LookupEnv(keyring.CredentialStoreEnv); set {
			t.Errorf("env should remain unset, got %q", got)
		}
	})

	t.Run("idempotent across multiple calls", func(t *testing.T) {
		// main() and factory.New() both invoke the bridge.
		withCredentialStoreEnv(t, "")

		conf := config.New(afero.NewMemMapFs(), nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		ApplyCredentialStoreFromConfig(conf)
		first := os.Getenv(keyring.CredentialStoreEnv)
		ApplyCredentialStoreFromConfig(conf)
		second := os.Getenv(keyring.CredentialStoreEnv)

		if first != keyring.StoreSHM || second != keyring.StoreSHM {
			t.Errorf("expected stable env=%q across calls, got first=%q second=%q",
				keyring.StoreSHM, first, second)
		}
	})

	t.Run("empty env falls through to config", func(t *testing.T) {
		// keyring.New() treats empty env as auto.
		withCredentialStoreEnv(t, "")
		_ = os.Setenv(keyring.CredentialStoreEnv, "")
		t.Cleanup(func() { _ = os.Unsetenv(keyring.CredentialStoreEnv) })

		conf := config.New(afero.NewMemMapFs(), nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		ApplyCredentialStoreFromConfig(conf)

		if got := os.Getenv(keyring.CredentialStoreEnv); got != keyring.StoreSHM {
			t.Errorf("env = %q, want %q", got, keyring.StoreSHM)
		}
	})
}

func TestNewUserAgent(t *testing.T) {
	t.Chdir(t.TempDir())

	t.Run("non-preflight factory does not set preflight suffix", func(t *testing.T) {
		f, err := New()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if strings.Contains(f.RestAPIClient.UserAgent, "buildkite-cli-preflight/") {
			t.Fatalf("expected no preflight suffix in %q", f.RestAPIClient.UserAgent)
		}
	})

	t.Run("factory can opt in to preflight suffix", func(t *testing.T) {
		f, err := New(WithUserAgentSuffix("buildkite-cli-preflight/3.x"))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if !strings.Contains(f.RestAPIClient.UserAgent, "buildkite-cli-preflight/3.x") {
			t.Fatalf("expected preflight suffix in %q", f.RestAPIClient.UserAgent)
		}
	})
}
