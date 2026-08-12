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
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/gorilla/websocket"
	"github.com/jpillora/chisel/share/cnet"
	"golang.org/x/crypto/ssh"
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
		fmt.Fprint(w, `{"endpoint":"wss://ssh.example.test/session","access_token":"namespace-access-token","expires_at":"2026-08-12T01:02:03Z","transport":"namespace_ingress","ssh":{"username":"root","private_key":"private-key"}}`)
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
		Transport: sshTransportNamespaceIngress,
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: "private-key",
		},
	}
	if got != want {
		t.Errorf("createSSHSession() = %#v, want %#v", got, want)
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
		Transport: sshTransportNamespaceIngress,
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
		{name: "missing access token", mutate: func(s *sshSession) { s.AccessToken = "" }, want: "without an access token"},
		{name: "missing expiry", mutate: func(s *sshSession) { s.ExpiresAt = time.Time{} }, want: "without an expiry"},
		{name: "missing transport", mutate: func(s *sshSession) { s.Transport = "" }, want: "without a transport"},
		{name: "unknown transport", mutate: func(s *sshSession) { s.Transport = "carrier_pigeon" }, want: `unsupported SSH transport "carrier_pigeon"`},
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

func TestSSHSessionTCPURLValidation(t *testing.T) {
	t.Parallel()

	valid := sshSession{
		remoteSession: remoteSession{
			Endpoint:    "tcp://ssh.example.test:22",
			AccessToken: "namespace-access-token",
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		Transport: sshTransportTCP,
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: "private-key",
		},
	}

	for _, endpoint := range []string{
		"tcp://ssh.example.test:22",
		"tcp://127.0.0.1:2222",
		"tcp://[2001:db8::1]:22",
	} {
		t.Run("valid "+endpoint, func(t *testing.T) {
			t.Parallel()

			session := valid
			session.Endpoint = endpoint
			if err := session.validate(); err != nil {
				t.Fatalf("validate() error = %v, want nil", err)
			}
		})
	}

	for _, endpoint := range []string{
		"ssh.example.test:22",
		"http://ssh.example.test:22",
		"tcp://ssh.example.test",
		"tcp://:22",
		"tcp://ssh.example.test:not-a-port",
		"tcp://ssh.example.test:+22",
		"tcp://ssh.example.test:0",
		"tcp://ssh.example.test:65536",
		"tcp://user@ssh.example.test:22",
		"tcp://ssh.example.test:22/",
		"tcp://ssh.example.test:22/path",
		"tcp://ssh.example.test:22?token=secret",
		"tcp://ssh.example.test:22#fragment",
		"tcp://ssh.example.test:22#",
	} {
		t.Run("invalid "+endpoint, func(t *testing.T) {
			t.Parallel()

			session := valid
			session.Endpoint = endpoint
			err := session.validate()
			if err == nil || !strings.Contains(err.Error(), "invalid TCP endpoint") {
				t.Fatalf("validate() error = %v, want invalid TCP endpoint error", err)
			}
		})
	}
}

func TestSSHCmdRunNamespaceIngress(t *testing.T) {
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
			Endpoint:    "ws" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		Transport: sshTransportNamespaceIngress,
		SSH: sshSessionCredentials{
			Username:   username,
			PrivateKey: privateKey,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := SSHCmd{JobID: "job-uuid"}
	if err := cmd.run(ctx, sshStreams{
		Stdin:  strings.NewReader(clientData),
		Stdout: &stdout,
		Stderr: &stderr,
	}, client, "buildkite"); err != nil {
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

func TestSSHCmdRunTCP(t *testing.T) {
	t.Parallel()

	const (
		accessToken = "namespace-access-token-must-not-be-sent"
		username    = "root"
		clientData  = "from local terminal"
		serverData  = "from hosted Linux container"
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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for SSH: %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- fmt.Errorf("accept TCP connection: %w", err)
			return
		}
		defer conn.Close()

		prefix := make([]byte, 4)
		if _, err := io.ReadFull(conn, prefix); err != nil {
			serverErr <- fmt.Errorf("read SSH identification prefix: %w", err)
			return
		}
		if got := string(prefix); got != "SSH-" {
			serverErr <- fmt.Errorf("TCP connection started with %q, want SSH identification without access token", got)
			return
		}

		serverErr <- serveSSHTestConnection(&prefixedConn{Conn: conn, prefix: bytes.NewReader(prefix)}, serverConfig, username, clientData, serverData)
	}()

	client := newSSHSessionTestClient(t, sshSession{
		remoteSession: remoteSession{
			Endpoint:    "tcp://" + listener.Addr().String(),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		Transport: sshTransportTCP,
		SSH: sshSessionCredentials{
			Username:   username,
			PrivateKey: privateKey,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := SSHCmd{JobID: "job-uuid"}
	if err := cmd.run(ctx, sshStreams{
		Stdin:  strings.NewReader(clientData),
		Stdout: &stdout,
		Stderr: &stderr,
	}, client, "buildkite"); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Error(err)
	}
	if got := stdout.String(); got != serverData {
		t.Errorf("stdout = %q, want %q", got, serverData)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
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
		Transport: sshTransportNamespaceIngress,
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
			Endpoint:    "ws" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		Transport: sshTransportNamespaceIngress,
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
		runErr <- cmd.run(ctx, sshStreams{
			Stdin:  stdinReader,
			Stdout: io.Discard,
			Stderr: io.Discard,
		}, client, "buildkite")
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
			Endpoint:    "ws" + strings.TrimPrefix(gateway.URL, "http"),
			AccessToken: accessToken,
			ExpiresAt:   time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC),
		},
		Transport: sshTransportNamespaceIngress,
		SSH: sshSessionCredentials{
			Username:   "root",
			PrivateKey: privateKey,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		cmd := SSHCmd{JobID: "job-uuid"}
		runErr <- cmd.run(ctx, sshStreams{
			Stdout: io.Discard,
			Stderr: io.Discard,
		}, client, "buildkite")
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
		fmt.Fprintf(w, `{"endpoint":%q,"access_token":%q,"expires_at":%q,"transport":%q,"ssh":{"username":%q,"private_key":%q}}`, session.Endpoint, session.AccessToken, session.ExpiresAt.Format(time.RFC3339), session.Transport, session.SSH.Username, session.SSH.PrivateKey)
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

type prefixedConn struct {
	net.Conn
	prefix io.Reader
}

func (c *prefixedConn) Read(p []byte) (int, error) {
	if c.prefix != nil {
		n, err := c.prefix.Read(p)
		if !errors.Is(err, io.EOF) {
			return n, err
		}
		c.prefix = nil
		if n > 0 {
			return n, nil
		}
	}
	return c.Conn.Read(p)
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
