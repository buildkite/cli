package job

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"namespacelabs.dev/integrations/api"
)

func TestCreateVNCSession(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-uuid/vnc-session" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("request body = %q, want empty", body)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"endpoint":"wss://vnc.example.test/session","access_token":"namespace-access-token","expires_at":"2026-08-10T01:02:03Z","vnc":{"username":"vnc-user","password":"vnc-password"}}`)
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := createVNCSession(context.Background(), client, "buildkite", "job-uuid")
	if err != nil {
		t.Fatalf("createVNCSession() error = %v", err)
	}

	want := vncSession{
		Endpoint:    "wss://vnc.example.test/session",
		AccessToken: "namespace-access-token",
		ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		VNC: vncSessionCredentials{
			Username: "vnc-user",
			Password: "vnc-password",
		},
	}
	if got != want {
		t.Errorf("createVNCSession() = %#v, want %#v", got, want)
	}
}

func TestVNCSessionValidation(t *testing.T) {
	t.Parallel()

	valid := vncSession{
		Endpoint:    "wss://vnc.example.test/session",
		AccessToken: "namespace-access-token",
		ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		VNC: vncSessionCredentials{
			Username: "vnc-user",
			Password: "vnc-password",
		},
	}

	tests := []struct {
		name   string
		mutate func(*vncSession)
		want   string
	}{
		{name: "missing endpoint", mutate: func(s *vncSession) { s.Endpoint = "" }, want: "without an endpoint"},
		{name: "missing access token", mutate: func(s *vncSession) { s.AccessToken = "" }, want: "without an access token"},
		{name: "missing expiry", mutate: func(s *vncSession) { s.ExpiresAt = time.Time{} }, want: "without an expiry"},
		{name: "missing username", mutate: func(s *vncSession) { s.VNC.Username = "" }, want: "without a VNC username"},
		{name: "missing password", mutate: func(s *vncSession) { s.VNC.Password = "" }, want: "without a VNC password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			session := valid
			test.mutate(&session)
			err := session.validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestVNCCmdRun(t *testing.T) {
	t.Parallel()

	remote, remotePeer := net.Pipe()
	defer remotePeer.Close()

	client := new(buildkite.Client)
	const (
		accessToken = "namespace-access-token"
		username    = "vnc-user"
		password    = "vnc-password"
	)
	var (
		gotOrganization string
		gotJobID        string
		gotEndpoint     string
		gotToken        string
		gotURL          string
		proxiedRemote   net.Conn
	)

	deps := vncDependencies{
		createSession: func(_ context.Context, gotClient *buildkite.Client, organization, jobID string) (vncSession, error) {
			if gotClient != client {
				t.Errorf("createSession client = %p, want %p", gotClient, client)
			}
			gotOrganization = organization
			gotJobID = jobID
			return vncSession{
				Endpoint:    "wss://vnc.example.test/session",
				AccessToken: accessToken,
				ExpiresAt:   time.Now().Add(time.Minute),
				VNC: vncSessionCredentials{
					Username: username,
					Password: password,
				},
			}, nil
		},
		dialEndpoint: func(ctx context.Context, token api.TokenSource, endpoint string) (net.Conn, error) {
			var err error
			gotToken, err = token.IssueToken(ctx, 30*time.Second, false)
			if err != nil {
				return nil, err
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
	cmd := VNCCmd{JobID: "job-uuid"}
	if err := cmd.run(context.Background(), &stdout, false, client, "buildkite", deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotOrganization != "buildkite" {
		t.Errorf("organization = %q, want buildkite", gotOrganization)
	}
	if gotJobID != "job-uuid" {
		t.Errorf("job ID = %q, want job-uuid", gotJobID)
	}
	if gotEndpoint != "wss://vnc.example.test/session" {
		t.Errorf("dialed endpoint = %q", gotEndpoint)
	}
	if gotToken != accessToken {
		t.Errorf("dial token = %q, want API access token", gotToken)
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
	if got := parsedURL.User.Username(); got != username {
		t.Errorf("VNC URL username = %q", got)
	}
	if got, ok := parsedURL.User.Password(); !ok || got != password {
		t.Errorf("VNC URL password = %q, present = %v", got, ok)
	}
	host, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatalf("split VNC URL host: %v", err)
	}
	if host != "127.0.0.1" {
		t.Errorf("VNC listener host = %q, want 127.0.0.1", host)
	}

	wantOutput := "Connected to job.\nOpening VNC client...\nClient connected.\nClient disconnected, leaving.\n"
	if stdout.String() != wantOutput {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
	for _, secret := range []string{accessToken, username, password} {
		if strings.Contains(stdout.String(), secret) {
			t.Errorf("stdout contains VNC session material: %q", stdout.String())
		}
	}
}

func TestVNCClientURLEscapesCredentials(t *testing.T) {
	t.Parallel()

	rawURL := vncClientURL("127.0.0.1:5900", "user@example.com", "pass:word/@")
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse VNC URL: %v", err)
	}
	if got := parsed.User.Username(); got != "user@example.com" {
		t.Errorf("username = %q", got)
	}
	if got, ok := parsed.User.Password(); !ok || got != "pass:word/@" {
		t.Errorf("password = %q, present = %v", got, ok)
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
