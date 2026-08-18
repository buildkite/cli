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
	"github.com/gorilla/websocket"
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
		remoteSession: remoteSession{
			Endpoint:    "wss://vnc.example.test/session",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		},
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
		remoteSession: remoteSession{
			Endpoint:    "wss://vnc.example.test/session",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		},
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

func newVNCSessionTestClient(t *testing.T, session vncSession) *buildkite.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-uuid/vnc-session" {
			t.Errorf("request path = %q", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"endpoint":%q,"access_token":%q,"expires_at":%q,"vnc":{"username":%q,"password":%q}}`, session.Endpoint, session.AccessToken, session.ExpiresAt.Format(time.RFC3339), session.VNC.Username, session.VNC.Password)
	}))
	t.Cleanup(server.Close)

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func TestVNCCmdRun(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "namespace-access-token"
		username    = "vnc-user"
		password    = "vnc-password"
		clientData  = "from local VNC client"
		serverData  = "from hosted VNC server"
	)

	gatewayErr := make(chan error, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			gatewayErr <- fmt.Errorf("Authorization = %q, want bearer VNC access token", got)
			return
		}

		conn, err := new(websocket.Upgrader).Upgrade(w, r, nil)
		if err != nil {
			gatewayErr <- fmt.Errorf("upgrade gateway connection: %w", err)
			return
		}
		defer conn.Close()

		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			gatewayErr <- fmt.Errorf("read proxied client data: %w", err)
			return
		}
		if messageType != websocket.BinaryMessage {
			gatewayErr <- fmt.Errorf("message type = %d, want binary", messageType)
			return
		}
		if string(payload) != clientData {
			gatewayErr <- fmt.Errorf("proxied client data = %q, want %q", payload, clientData)
			return
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(serverData)); err != nil {
			gatewayErr <- fmt.Errorf("write proxied server data: %w", err)
			return
		}

		// Keep the gateway connection open until the local VNC client disconnects.
		_, _, _ = conn.ReadMessage()
		gatewayErr <- nil
	}))
	defer gateway.Close()

	client := newVNCSessionTestClient(t, vncSession{
		remoteSession: remoteSession{
			Endpoint:    "ws" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		},
		VNC: vncSessionCredentials{
			Username: username,
			Password: password,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var gotURL string
	openURL := func(rawURL string) error {
		gotURL = rawURL
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}

		conn, err := net.Dial("tcp", parsed.Host)
		if err != nil {
			return err
		}
		defer conn.Close()

		if _, err := io.WriteString(conn, clientData); err != nil {
			return fmt.Errorf("write local VNC data: %w", err)
		}
		response := make([]byte, len(serverData))
		if _, err := io.ReadFull(conn, response); err != nil {
			return fmt.Errorf("read local VNC data: %w", err)
		}
		if string(response) != serverData {
			return fmt.Errorf("proxied server data = %q, want %q", response, serverData)
		}

		return nil
	}

	var stdout bytes.Buffer
	cmd := VNCCmd{JobID: "job-uuid"}
	if err := cmd.run(ctx, &stdout, false, client, "buildkite", openURL); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("run did not finish before its timeout: %v", ctx.Err())
	}

	if err := <-gatewayErr; err != nil {
		t.Error(err)
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

func TestVNCCmdRunGatewayDisconnect(t *testing.T) {
	t.Parallel()

	const accessToken = "namespace-access-token"

	gatewayErr := make(chan error, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			gatewayErr <- fmt.Errorf("Authorization = %q, want bearer VNC access token", got)
			return
		}

		conn, err := new(websocket.Upgrader).Upgrade(w, r, nil)
		if err != nil {
			gatewayErr <- fmt.Errorf("upgrade gateway connection: %w", err)
			return
		}
		defer conn.Close()

		if err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "job finished"),
			time.Now().Add(time.Second),
		); err != nil {
			gatewayErr <- fmt.Errorf("close gateway connection: %w", err)
			return
		}
		gatewayErr <- nil
	}))
	defer gateway.Close()

	client := newVNCSessionTestClient(t, vncSession{
		remoteSession: remoteSession{
			Endpoint:    "ws" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC),
		},
		VNC: vncSessionCredentials{
			Username: "vnc-user",
			Password: "vnc-password",
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	openURL := func(rawURL string) error {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}

		conn, err := net.Dial("tcp", parsed.Host)
		if err != nil {
			return err
		}
		defer conn.Close()

		_, err = io.Copy(io.Discard, conn)
		return err
	}

	var stdout bytes.Buffer
	cmd := VNCCmd{JobID: "job-uuid"}
	if err := cmd.run(ctx, &stdout, false, client, "buildkite", openURL); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("run did not finish before its timeout: %v", ctx.Err())
	}
	if err := <-gatewayErr; err != nil {
		t.Error(err)
	}

	wantOutput := "Connected to job.\nOpening VNC client...\nClient connected.\nClient disconnected, leaving.\n"
	if stdout.String() != wantOutput {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
}

func TestIsExpectedVNCDisconnect(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "context canceled", ctx: canceledCtx, err: fmt.Errorf("proxy failed"), want: true},
		{name: "connection closed", ctx: context.Background(), err: fmt.Errorf("proxy: %w", net.ErrClosed), want: true},
		{name: "normal WebSocket close", ctx: context.Background(), err: fmt.Errorf("proxy: %w", &websocket.CloseError{Code: websocket.CloseNormalClosure}), want: true},
		{name: "WebSocket going away", ctx: context.Background(), err: fmt.Errorf("proxy: %w", &websocket.CloseError{Code: websocket.CloseGoingAway}), want: true},
		{name: "unexpected WebSocket close", ctx: context.Background(), err: fmt.Errorf("proxy: %w", &websocket.CloseError{Code: websocket.CloseInternalServerErr}), want: false},
		{name: "other proxy error", ctx: context.Background(), err: fmt.Errorf("proxy failed"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := isExpectedVNCDisconnect(test.ctx, test.err); got != test.want {
				t.Errorf("isExpectedVNCDisconnect() = %t, want %t", got, test.want)
			}
		})
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
