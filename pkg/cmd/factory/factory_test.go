package factory

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/keyring"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/spf13/afero"
)

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

func TestDebugTransportRedactsClusterSecretValues(t *testing.T) {
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
			secretMarkers: []string{"secret-prefix", "quoted", "secret-suffix"},
			wantVisible:   `"key":"API_KEY"`,
		},
		{
			name:          "update secret value",
			method:        http.MethodPut,
			path:          "/v2/organizations/test/clusters/cluster-1/secrets/secret-1/value",
			body:          `{ "value" : "replacement-secret" }`,
			secretMarkers: []string{"replacement-secret"},
		},
		{
			name:          "malformed cluster ID still redacts",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1//secrets",
			body:          `{"key":"API_KEY","value":"malformed-id-secret"}`,
			secretMarkers: []string{"malformed-id-secret"},
			wantVisible:   `"key":"API_KEY"`,
		},
		{
			name:          "query delimiter in cluster ID still redacts",
			method:        http.MethodPost,
			path:          "/v2/organizations/test/clusters/cluster-1?/secrets",
			body:          `{"key":"API_KEY","value":"query-delimiter-secret"}`,
			secretMarkers: []string{"query-delimiter-secret"},
			wantVisible:   `"key":"API_KEY"`,
		},
		{
			name:          "fragment delimiter in secret ID still redacts",
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
			if len(test.secretMarkers) > 0 && !strings.Contains(stderr, "[REDACTED]") {
				t.Errorf("stderr does not contain a redaction marker:\n%s", stderr)
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
