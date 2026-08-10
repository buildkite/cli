package job

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"namespacelabs.dev/integrations/api"
)

type recordingTokenSource struct {
	token       string
	minDuration time.Duration
	force       bool
}

func (s *recordingTokenSource) IssueToken(_ context.Context, minDuration time.Duration, force bool) (string, error) {
	s.minDuration = minDuration
	s.force = force
	return s.token, nil
}

func TestGetVNCService(t *testing.T) {
	const (
		instanceID = "i_test"
		bearer     = "secret-namespace-token"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/"+getKubernetesClusterMethod {
			t.Errorf("request path = %q, want %q", r.URL.Path, "/"+getKubernetesClusterMethod)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+bearer {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("NS-Internal-Version"); got != namespaceAPIVersion {
			t.Errorf("NS-Internal-Version = %q, want %q", got, namespaceAPIVersion)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var request struct {
			ClusterID string `json:"cluster_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.ClusterID != instanceID {
			t.Errorf("cluster_id = %q, want %q", request.ClusterID, instanceID)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"cluster":{"service_state":[{"name":"ssh","status":"READY"},{"name":"vnc","status":"READY","endpoint":"wss://vnc.example.test","credentials":{"username":"vnc-user","password":"vnc-password"}}]}}`)
	}))
	defer server.Close()

	t.Setenv("NSC_ENDPOINT", server.URL)
	token := &recordingTokenSource{token: bearer}
	service, err := getVNCService(context.Background(), server.Client(), token, instanceID)
	if err != nil {
		t.Fatalf("getVNCService() error = %v", err)
	}

	if token.minDuration != 15*time.Minute {
		t.Errorf("token minimum duration = %s, want 15m", token.minDuration)
	}
	if token.force {
		t.Error("token was issued with force=true, want false")
	}
	if service.Endpoint != "wss://vnc.example.test" {
		t.Errorf("service endpoint = %q", service.Endpoint)
	}
	if service.Credentials == nil {
		t.Fatal("service credentials are nil")
	}
	if service.Credentials.Username != "vnc-user" || service.Credentials.Password != "vnc-password" {
		t.Errorf("service credentials = %#v", service.Credentials)
	}
}

func TestGetVNCServiceValidation(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "missing instance",
			response: `{}`,
			want:     `instance "i_test" was not returned`,
		},
		{
			name:     "missing VNC service",
			response: `{"cluster":{"service_state":[]}}`,
			want:     `does not have a VNC service`,
		},
		{
			name:     "VNC service not ready",
			response: `{"cluster":{"service_state":[{"name":"vnc","status":"STARTING","endpoint":"wss://example.test"}]}}`,
			want:     `is not ready (status: STARTING)`,
		},
		{
			name:     "VNC endpoint missing",
			response: `{"cluster":{"service_state":[{"name":"vnc","status":"READY"}]}}`,
			want:     `has no endpoint`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, test.response)
			}))
			defer server.Close()

			t.Setenv("NSC_ENDPOINT", server.URL)
			_, err := getVNCService(context.Background(), server.Client(), &recordingTokenSource{token: "token"}, "i_test")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("getVNCService() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestVNCCmdRun(t *testing.T) {
	t.Parallel()

	remote, remotePeer := net.Pipe()
	defer remotePeer.Close()

	token := &recordingTokenSource{token: "token"}
	credentials := &vncCredentials{Username: "vnc-user", Password: "vnc-password"}
	var (
		gotInstanceID string
		gotEndpoint   string
		gotURL        string
		proxiedRemote net.Conn
	)

	deps := vncDependencies{
		loadToken: func() (api.TokenSource, error) {
			return token, nil
		},
		getService: func(_ context.Context, _ *http.Client, gotToken api.TokenSource, instanceID string) (*vncService, error) {
			if gotToken != token {
				t.Errorf("getService token = %T, want recording token", gotToken)
			}
			gotInstanceID = instanceID
			return &vncService{
				Name:        "vnc",
				Status:      "READY",
				Endpoint:    "wss://vnc.example.test",
				Credentials: credentials,
			}, nil
		},
		dialEndpoint: func(_ context.Context, gotToken api.TokenSource, endpoint string) (net.Conn, error) {
			if gotToken != token {
				t.Errorf("dialEndpoint token = %T, want recording token", gotToken)
			}
			gotEndpoint = endpoint
			return remote, nil
		},
		listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			return new(net.ListenConfig).Listen(ctx, network, address)
		},
		openURL: func(rawURL string) error {
			gotURL = rawURL
			parsed, err := url.Parse(rawURL)
			if err != nil {
				return err
			}
			conn, err := net.Dial("tcp", parsed.Host)
			if err != nil {
				return err
			}
			return conn.Close()
		},
		proxy: func(local, gotRemote net.Conn) error {
			proxiedRemote = gotRemote
			return nil
		},
	}

	var stdout bytes.Buffer
	cmd := VNCCmd{InstanceID: "i_test"}
	if err := cmd.run(context.Background(), &stdout, false, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotInstanceID != "i_test" {
		t.Errorf("instance ID = %q, want i_test", gotInstanceID)
	}
	if gotEndpoint != "wss://vnc.example.test" {
		t.Errorf("dialed endpoint = %q", gotEndpoint)
	}
	if proxiedRemote != remote {
		t.Error("proxy did not receive the ingress connection")
	}

	parsedURL, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("parse VNC URL: %v", err)
	}
	if parsedURL.Scheme != "vnc" {
		t.Errorf("VNC URL scheme = %q", parsedURL.Scheme)
	}
	if username := parsedURL.User.Username(); username != credentials.Username {
		t.Errorf("VNC URL username = %q", username)
	}
	if password, ok := parsedURL.User.Password(); !ok || password != credentials.Password {
		t.Errorf("VNC URL password = %q, present = %v", password, ok)
	}
	host, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatalf("split VNC URL host: %v", err)
	}
	if host != "127.0.0.1" {
		t.Errorf("VNC listener host = %q, want 127.0.0.1", host)
	}

	wantOutput := "Connected to instance.\nOpening VNC client...\nClient connected.\nClient disconnected, leaving.\n"
	if stdout.String() != wantOutput {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
	if strings.Contains(stdout.String(), credentials.Username) || strings.Contains(stdout.String(), credentials.Password) {
		t.Errorf("stdout contains VNC credentials: %q", stdout.String())
	}
}

func TestVNCClientURLDefaultsAndEscapesCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		credentials *vncCredentials
		wantUser    string
		wantPass    string
	}{
		{name: "nsc defaults", wantUser: "admin", wantPass: "admin"},
		{
			name:        "service credentials",
			credentials: &vncCredentials{Username: "user@example.com", Password: "pass:word/@"},
			wantUser:    "user@example.com",
			wantPass:    "pass:word/@",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rawURL := vncClientURL("127.0.0.1:5900", test.credentials)
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatalf("parse VNC URL: %v", err)
			}
			if got := parsed.User.Username(); got != test.wantUser {
				t.Errorf("username = %q, want %q", got, test.wantUser)
			}
			if got, ok := parsed.User.Password(); !ok || got != test.wantPass {
				t.Errorf("password = %q, present = %v, want %q", got, ok, test.wantPass)
			}
		})
	}
}

func TestNamespaceComputeEndpoint(t *testing.T) {
	t.Setenv("NSC_ENDPOINT", "")

	tests := []struct {
		name   string
		claims map[string]string
		want   string
	}{
		{
			name:   "workload region",
			claims: map[string]string{"workload_region": "au", "primary_region": "us"},
			want:   "https://au.compute.namespaceapis.com",
		},
		{
			name:   "primary region",
			claims: map[string]string{"primary_region": "us"},
			want:   "https://us.compute.namespaceapis.com",
		},
		{
			name: "nsc default region",
			want: "https://eu.compute.namespaceapis.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := namespaceComputeEndpoint(unsignedNamespaceToken(t, test.claims))
			if err != nil {
				t.Fatalf("namespaceComputeEndpoint() error = %v", err)
			}
			if got != test.want {
				t.Errorf("namespaceComputeEndpoint() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWriteVNCStatusQuiet(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	writeVNCStatus(&stdout, true, "secret status")
	if stdout.Len() != 0 {
		t.Errorf("quiet status output = %q, want empty", stdout.String())
	}
}

func unsignedNamespaceToken(t *testing.T, claims map[string]string) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal token header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal token claims: %v", err)
	}

	return "nsct_" + base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("signature"))
}
