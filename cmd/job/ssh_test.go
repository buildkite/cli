package job

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/gorilla/websocket"
	"github.com/jpillora/chisel/share/cnet"
	"golang.org/x/crypto/ssh"
	"namespacelabs.dev/integrations/nsc/ingress"
)

func TestCreateSSHSession(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-uuid/ssh-session" {
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
		fmt.Fprint(w, `{"endpoint":"wss://ssh.example.test/session","access_token":"namespace-access-token","expires_at":"2026-08-12T01:02:03Z","ssh":{"username":"root","private_key":"private-key"}}`)
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := createSSHSession(context.Background(), client, "buildkite", "job-uuid")
	if err != nil {
		t.Fatalf("createSSHSession() error = %v", err)
	}

	want := sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss://ssh.example.test/session",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: "private-key",
		},
	}
	if got != want {
		t.Errorf("createSSHSession() = %#v, want %#v", got, want)
	}
}

func TestSSHCmdHelpMentionsMacOS(t *testing.T) {
	t.Parallel()

	if help := new(SSHCmd).Help(); !strings.Contains(help, "hosted macOS job") {
		t.Errorf("Help() = %q, want macOS scope", help)
	}
}

func TestSSHSessionValidation(t *testing.T) {
	t.Parallel()

	valid := sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss://ssh.example.test/session",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: "private-key",
		},
	}

	tests := []struct {
		name   string
		mutate func(*sshSession)
		want   string
	}{
		{name: "missing endpoint", mutate: func(s *sshSession) { s.Endpoint = "" }, want: "without an endpoint"},
		{name: "invalid endpoint", mutate: func(s *sshSession) { s.Endpoint = "://" }, want: "invalid endpoint"},
		{name: "insecure websocket endpoint", mutate: func(s *sshSession) { s.Endpoint = "ws://ssh.example.test/session" }, want: `must use "wss"`},
		{name: "TCP endpoint", mutate: func(s *sshSession) { s.Endpoint = "tcp://ssh.example.test:22" }, want: `must use "wss"`},
		{name: "missing endpoint hostname", mutate: func(s *sshSession) { s.Endpoint = "wss:///session" }, want: "without a hostname"},
		{name: "missing access token", mutate: func(s *sshSession) { s.AccessToken = "" }, want: "without an access token"},
		{name: "missing expiry", mutate: func(s *sshSession) { s.ExpiresAt = time.Time{} }, want: "without an expiry"},
		{name: "missing username", mutate: func(s *sshSession) { s.SSH.Username = "" }, want: "without an SSH username"},
		{name: "missing private key", mutate: func(s *sshSession) { s.SSH.PrivateKey = "" }, want: "without an SSH private key"},
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

func TestSSHCmdRun(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "namespace-access-token"
		username    = "root"
		clientData  = "from local terminal"
		serverData  = "from hosted SSH server"
	)

	clientSigner, privateKey := newSSHTestKey(t)
	hostSigner, _ := newSSHTestKey(t)

	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), clientSigner.PublicKey().Marshal()) {
				return nil, errors.New("unexpected SSH public key")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostSigner)

	gatewayErr := make(chan error, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			gatewayErr <- fmt.Errorf("Authorization = %q, want bearer SSH access token", got)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		ws, err := new(websocket.Upgrader).Upgrade(w, r, nil)
		if err != nil {
			gatewayErr <- fmt.Errorf("upgrade gateway connection: %w", err)
			return
		}
		gatewayErr <- serveSSHTestConnection(cnet.NewWebSocketConn(ws), serverConfig, username, clientData, serverData)
	}))
	defer gateway.Close()

	client := newSSHSessionTestClient(t, sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   username,
			PrivateKey: privateKey,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := SSHCmd{JobID: "job-uuid"}
	if err := cmd.runWithDialer(ctx, sshStreams{
		Stdin:  strings.NewReader(clientData),
		Stdout: &stdout,
		Stderr: &stderr,
	}, client, "buildkite", dialInsecureTestSSHEndpoint); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("run did not finish before its timeout: %v", ctx.Err())
	}

	if err := <-gatewayErr; err != nil {
		t.Error(err)
	}
	if got := stdout.String(); got != serverData {
		t.Errorf("stdout = %q, want %q", got, serverData)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	for _, secret := range []string{accessToken, privateKey} {
		if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Errorf("command output contains SSH session material")
		}
	}
}

func TestSSHCmdRunInvalidPrivateKey(t *testing.T) {
	t.Parallel()

	client := newSSHSessionTestClient(t, sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss://ssh.example.test/session",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: "not-a-private-key",
		},
	})

	cmd := SSHCmd{JobID: "job-uuid"}
	err := cmd.run(context.Background(), sshStreams{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
	}, client, "buildkite")
	if err == nil || !strings.Contains(err.Error(), "parse SSH private key") {
		t.Fatalf("run() error = %v, want private key parse error", err)
	}
	if strings.Contains(err.Error(), "not-a-private-key") {
		t.Fatalf("run() error contains private key material: %v", err)
	}
}

func TestSSHCmdRunCancellation(t *testing.T) {
	t.Parallel()

	const accessToken = "namespace-access-token"

	clientSigner, privateKey := newSSHTestKey(t)
	hostSigner, _ := newSSHTestKey(t)
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), clientSigner.PublicKey().Marshal()) {
				return nil, errors.New("unexpected SSH public key")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostSigner)

	shellStarted := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		ws, err := new(websocket.Upgrader).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serveBlockingSSHTestConnection(cnet.NewWebSocketConn(ws), serverConfig, shellStarted)
	}))
	defer gateway.Close()

	client := newSSHSessionTestClient(t, sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: privateKey,
		},
	})

	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()
	defer stdinWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		cmd := SSHCmd{JobID: "job-uuid"}
		runErr <- cmd.runWithDialer(ctx, sshStreams{
			Stdin:  stdinReader,
			Stdout: io.Discard,
			Stderr: io.Discard,
		}, client, "buildkite", dialInsecureTestSSHEndpoint)
	}()

	select {
	case <-shellStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("SSH shell did not start")
	}
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish after cancellation")
	}
}

func TestSSHCmdRunCancellationDuringHandshake(t *testing.T) {
	t.Parallel()

	const accessToken = "namespace-access-token"

	_, privateKey := newSSHTestKey(t)
	handshakeStarted := make(chan struct{})
	releaseServer := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		ws, err := new(websocket.Upgrader).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := cnet.NewWebSocketConn(ws)
		defer conn.Close()

		identificationPrefix := make([]byte, 4)
		if _, err := io.ReadFull(conn, identificationPrefix); err != nil {
			return
		}
		close(handshakeStarted)
		<-releaseServer
	}))
	defer func() {
		close(releaseServer)
		gateway.Close()
	}()

	client := newSSHSessionTestClient(t, sshSession{
		remoteSession: remoteSession{
			Endpoint:    "wss" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: privateKey,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		cmd := SSHCmd{JobID: "job-uuid"}
		runErr <- cmd.runWithDialer(ctx, sshStreams{
			Stdout: io.Discard,
			Stderr: io.Discard,
		}, client, "buildkite", dialInsecureTestSSHEndpoint)
	}()

	select {
	case <-handshakeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("SSH handshake did not start")
	}
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run() after cancellation error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish after cancellation during the SSH handshake")
	}
}

func TestIsExpectedSSHExit(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "context canceled", ctx: canceledCtx, err: errors.New("connection closed"), want: true},
		{name: "remote exit", ctx: context.Background(), err: fmt.Errorf("wait: %w", &ssh.ExitError{}), want: true},
		{name: "missing remote exit status", ctx: context.Background(), err: fmt.Errorf("wait: %w", &ssh.ExitMissingError{}), want: true},
		{name: "other wait error", ctx: context.Background(), err: errors.New("connection failed"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := isExpectedSSHExit(test.ctx, test.err); got != test.want {
				t.Errorf("isExpectedSSHExit() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSSHPTYAvailable(t *testing.T) {
	t.Parallel()

	terminalInput, err := os.CreateTemp(t.TempDir(), "terminal-input")
	if err != nil {
		t.Fatalf("create terminal input: %v", err)
	}
	t.Cleanup(func() { _ = terminalInput.Close() })

	terminalSize, err := os.CreateTemp(t.TempDir(), "terminal-size")
	if err != nil {
		t.Fatalf("create terminal size handle: %v", err)
	}
	t.Cleanup(func() { _ = terminalSize.Close() })

	tests := []struct {
		name            string
		input           *os.File
		size            *os.File
		inputIsTerminal bool
		sizeIsTerminal  bool
		want            bool
	}{
		{name: "interactive input and output", input: terminalInput, size: terminalSize, inputIsTerminal: true, sizeIsTerminal: true, want: true},
		{name: "redirected input", input: terminalInput, size: terminalSize, sizeIsTerminal: true, want: false},
		{name: "redirected output", input: terminalInput, size: terminalSize, inputIsTerminal: true, want: false},
		{name: "missing input", size: terminalSize, sizeIsTerminal: true, want: false},
		{name: "missing output", input: terminalInput, inputIsTerminal: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			isTerminal := func(fd int) bool {
				switch fd {
				case int(terminalInput.Fd()):
					return test.inputIsTerminal
				case int(terminalSize.Fd()):
					return test.sizeIsTerminal
				default:
					t.Fatalf("unexpected file descriptor %d", fd)
					return false
				}
			}

			if got := sshPTYAvailable(test.input, test.size, isTerminal); got != test.want {
				t.Errorf("sshPTYAvailable() = %t, want %t", got, test.want)
			}
		})
	}
}

func newSSHSessionTestClient(t *testing.T, session sshSession) *buildkite.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-uuid/ssh-session" {
			t.Errorf("request path = %q", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"endpoint":%q,"access_token":%q,"expires_at":%q,"ssh":{"username":%q,"private_key":%q}}`, session.Endpoint, session.AccessToken, session.ExpiresAt.Format(time.RFC3339), session.SSH.Username, session.SSH.PrivateKey)
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

func dialInsecureTestSSHEndpoint(ctx context.Context, token remoteAccessToken, endpoint string) (net.Conn, error) {
	if !strings.HasPrefix(endpoint, "wss://") {
		return nil, fmt.Errorf("test SSH endpoint = %q, want wss:// endpoint", endpoint)
	}
	// The API fixture advertises wss:// so the production validation is exercised.
	// Rewrite it to ws:// only when dialing the local test server, avoiding test
	// certificate setup while keeping plaintext WebSockets out of production.
	return ingress.DialEndpoint(ctx, io.Discard, token, "ws://"+strings.TrimPrefix(endpoint, "wss://"))
}

func newSSHTestKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate SSH key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create SSH signer: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		t.Fatalf("marshal SSH private key: %v", err)
	}
	return signer, string(pem.EncodeToMemory(block))
}

func serveSSHTestConnection(conn net.Conn, config *ssh.ServerConfig, username, clientData, serverData string) error {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return fmt.Errorf("accept SSH connection: %w", err)
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)

	if got := serverConn.User(); got != username {
		return fmt.Errorf("SSH username = %q, want %q", got, username)
	}

	newChannel, ok := <-channels
	if !ok {
		return errors.New("SSH client did not open a channel")
	}
	if got := newChannel.ChannelType(); got != "session" {
		return fmt.Errorf("SSH channel type = %q, want session", got)
	}
	channel, channelRequests, err := newChannel.Accept()
	if err != nil {
		return fmt.Errorf("accept SSH session channel: %w", err)
	}
	defer channel.Close()

	for request := range channelRequests {
		if request.Type != "shell" {
			_ = request.Reply(false, nil)
			continue
		}
		if err := request.Reply(true, nil); err != nil {
			return fmt.Errorf("accept SSH shell: %w", err)
		}

		input, err := io.ReadAll(channel)
		if err != nil {
			return fmt.Errorf("read SSH stdin: %w", err)
		}
		if got := string(input); got != clientData {
			return fmt.Errorf("SSH stdin = %q, want %q", got, clientData)
		}
		if _, err := io.WriteString(channel, serverData); err != nil {
			return fmt.Errorf("write SSH stdout: %w", err)
		}
		_, err = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
		return err
	}

	return errors.New("SSH client did not request a shell")
}

func serveBlockingSSHTestConnection(conn net.Conn, config *ssh.ServerConfig, shellStarted chan<- struct{}) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)

	newChannel, ok := <-channels
	if !ok || newChannel.ChannelType() != "session" {
		return
	}
	channel, channelRequests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close()

	for request := range channelRequests {
		if request.Type != "shell" {
			_ = request.Reply(false, nil)
			continue
		}
		if request.Reply(true, nil) == nil {
			close(shellStarted)
			_, _ = io.Copy(io.Discard, channel)
		}
		return
	}
}
