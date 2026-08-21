package job

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"namespacelabs.dev/integrations/nsc/ingress"
)

type SSHCmd struct {
	JobID string `arg:"" name:"job-uuid" help:"UUID of the hosted agent job" required:""`
}

type sshSession struct {
	remoteSession
	Transport sshTransport          `json:"transport"`
	SSH       sshSessionCredentials `json:"ssh"`
}

type sshTransport string

const (
	sshTransportTCP              sshTransport = "tcp"
	sshTransportNamespaceIngress sshTransport = "namespace_ingress"
)

type sshSessionCredentials struct {
	Username   string   `json:"username"`
	PrivateKey string   `json:"private_key"`
	HostKeys   []string `json:"host_keys"`
}

func (s sshSession) validate() error {
	if s.Endpoint == "" {
		return errors.New("buildkite API returned an SSH session without an endpoint")
	}
	if s.ExpiresAt.IsZero() {
		return errors.New("buildkite API returned an SSH session without an expiry")
	}

	switch s.Transport {
	case "":
		return errors.New("buildkite API returned an SSH session without a transport")
	case sshTransportTCP:
		if _, err := sshTCPEndpointAddress(s.Endpoint); err != nil {
			return fmt.Errorf("buildkite API returned an SSH session with an invalid TCP endpoint: %w", err)
		}
		if len(s.SSH.HostKeys) == 0 {
			return errors.New("buildkite API returned a TCP SSH session without SSH host keys")
		}
	case sshTransportNamespaceIngress:
		if s.AccessToken == "" {
			return errors.New("buildkite API returned an SSH session without an access token")
		}
		endpoint, err := url.Parse(s.Endpoint)
		if err != nil {
			return fmt.Errorf("buildkite API returned an SSH session with an invalid endpoint: %w", err)
		}
		if endpoint.Scheme != "wss" {
			return fmt.Errorf("buildkite API returned an SSH session endpoint that must use %q, got %q", "wss", endpoint.Scheme)
		}
		if endpoint.Hostname() == "" {
			return errors.New("buildkite API returned an SSH session endpoint without a hostname")
		}
	default:
		return fmt.Errorf("buildkite API returned unsupported SSH transport %q", s.Transport)
	}

	if s.SSH.Username == "" {
		return errors.New("buildkite API returned an SSH session without an SSH username")
	}
	if s.SSH.PrivateKey == "" {
		return errors.New("buildkite API returned an SSH session without an SSH private key")
	}
	if _, err := parseSSHHostKeys(s.SSH.HostKeys); err != nil {
		return fmt.Errorf("buildkite API returned an SSH session with invalid SSH host keys: %w", err)
	}

	return nil
}

func (c *SSHCmd) Help() string {
	return `
Examples:
	# Open an interactive shell on a running hosted job
	$ bk job ssh 0190046e-e199-453b-a302-a21a4d649d31
`
}

func (c *SSHCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	organization, err := configuredOrganization(f.Config.OrganizationSlug())
	if err != nil {
		return err
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return c.run(ctx, sshStreams{
		Stdin:         os.Stdin,
		Stdout:        kongCtx.Stdout,
		Stderr:        kongCtx.Stderr,
		TerminalInput: os.Stdin,
		TerminalSize:  terminalSizeFile(os.Stdin, os.Stdout),
	}, f.RestAPIClient, organization)
}

type sshStreams struct {
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	TerminalInput *os.File
	TerminalSize  *os.File
}

func (c *SSHCmd) run(ctx context.Context, streams sshStreams, client *buildkite.Client, organization string) error {
	return c.runWithDialer(ctx, streams, client, organization, dialSSHSession)
}

type sshSessionDialer func(context.Context, sshSession) (net.Conn, error)

func dialSSHSession(ctx context.Context, session sshSession) (net.Conn, error) {
	switch session.Transport {
	case sshTransportTCP:
		address, err := sshTCPEndpointAddress(session.Endpoint)
		if err != nil {
			return nil, err
		}
		return new(net.Dialer).DialContext(ctx, "tcp", address)
	case sshTransportNamespaceIngress:
		return ingress.DialEndpoint(ctx, io.Discard, remoteAccessToken(session.AccessToken), session.Endpoint)
	default:
		return nil, fmt.Errorf("unsupported SSH transport %q", session.Transport)
	}
}

func (c *SSHCmd) runWithDialer(ctx context.Context, streams sshStreams, client *buildkite.Client, organization string, dialSession sshSessionDialer) error {
	session, err := createSSHSession(ctx, client, organization, c.JobID)
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(session.SSH.PrivateKey))
	if err != nil {
		return fmt.Errorf("parse SSH private key: %w", err)
	}

	hostKeyCallback, err := sshHostKeyCallback(session)
	if err != nil {
		return err
	}

	remote, err := dialSession(ctx, session)
	if err != nil {
		return fmt.Errorf("connect to the SSH service: %w", err)
	}
	defer remote.Close()
	stopRemoteClose := context.AfterFunc(ctx, func() {
		_ = remote.Close()
	})
	defer stopRemoteClose()

	config := &ssh.ClientConfig{
		User:            session.SSH.Username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
	}

	connection, channels, requests, err := ssh.NewClientConn(remote, remote.RemoteAddr().String(), config)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("negotiate SSH connection: %w", err)
	}
	sshClient := ssh.NewClient(connection, channels, requests)
	defer sshClient.Close()

	stopClose := context.AfterFunc(ctx, func() {
		_ = sshClient.Close()
	})
	defer stopClose()

	sshShell, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH shell: %w", err)
	}
	defer sshShell.Close()

	sshShell.Stdout = streams.Stdout
	sshShell.Stderr = streams.Stderr
	var remoteStdin io.WriteCloser
	if streams.Stdin != nil {
		remoteStdin, err = sshShell.StdinPipe()
		if err != nil {
			return fmt.Errorf("open SSH stdin: %w", err)
		}
	}

	restoreTerminal, err := configureSSHPTY(ctx, sshShell, streams.TerminalInput, streams.TerminalSize)
	if err != nil {
		return err
	}
	defer restoreTerminal()

	if err := sshShell.Shell(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("start SSH shell: %w", err)
	}
	if remoteStdin != nil {
		go func() {
			_, _ = io.Copy(remoteStdin, streams.Stdin)
			_ = remoteStdin.Close()
		}()
	}
	if err := sshShell.Wait(); err != nil {
		if isExpectedSSHExit(ctx, err) {
			return nil
		}
		return fmt.Errorf("wait for SSH shell: %w", err)
	}

	return nil
}

func sshHostKeyCallback(session sshSession) (ssh.HostKeyCallback, error) {
	if len(session.SSH.HostKeys) == 0 {
		if session.Transport != sshTransportNamespaceIngress {
			return nil, errors.New("refusing to connect without SSH host keys over an unauthenticated transport")
		}
		// Namespace ingress authenticates the endpoint using WebPKI and the
		// instance-scoped bearer token before any SSH traffic is exchanged.
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}

	hostKeys, err := parseSSHHostKeys(session.SSH.HostKeys)
	if err != nil {
		return nil, fmt.Errorf("parse SSH host keys: %w", err)
	}

	return func(_ string, _ net.Addr, presented ssh.PublicKey) error {
		for _, expected := range hostKeys {
			if bytes.Equal(presented.Marshal(), expected.Marshal()) {
				return nil
			}
		}
		return errors.New("SSH host key does not match any key provided by the Buildkite API")
	}, nil
}

func parseSSHHostKeys(encoded []string) ([]ssh.PublicKey, error) {
	hostKeys := make([]ssh.PublicKey, 0, len(encoded))
	for index, encodedKey := range encoded {
		hostKey, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(encodedKey))
		if err != nil {
			return nil, fmt.Errorf("host key %d is not a valid OpenSSH public key", index+1)
		}
		if len(bytes.TrimSpace(rest)) != 0 {
			return nil, fmt.Errorf("host key %d contains more than one public key", index+1)
		}
		hostKeys = append(hostKeys, hostKey)
	}
	return hostKeys, nil
}

func sshTCPEndpointAddress(endpoint string) (string, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", errors.New("URL could not be parsed")
	}
	if parsedURL.Scheme != "tcp" {
		return "", fmt.Errorf("scheme must be tcp, got %q", parsedURL.Scheme)
	}
	if parsedURL.User != nil {
		return "", errors.New("userinfo is not allowed")
	}
	if parsedURL.Path != "" || parsedURL.RawPath != "" {
		return "", errors.New("path is not allowed")
	}
	if parsedURL.RawQuery != "" || parsedURL.ForceQuery {
		return "", errors.New("query is not allowed")
	}
	if parsedURL.Fragment != "" || strings.Contains(endpoint, "#") {
		return "", errors.New("fragment is not allowed")
	}

	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return "", fmt.Errorf("host and port are required: %w", err)
	}
	if host == "" {
		return "", errors.New("host is required")
	}
	if strings.IndexFunc(port, func(character rune) bool {
		return character < '0' || character > '9'
	}) != -1 {
		return "", fmt.Errorf("port must be numeric, got %q", port)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535, got %q", port)
	}

	return parsedURL.Host, nil
}

func isExpectedSSHExit(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}

	var missingErr *ssh.ExitMissingError
	return errors.As(err, &missingErr)
}

func configureSSHPTY(ctx context.Context, session *ssh.Session, terminalInput, terminalSize *os.File) (func(), error) {
	if !sshPTYAvailable(terminalInput, terminalSize, term.IsTerminal) {
		return func() {}, nil
	}

	inputFD := int(terminalInput.Fd())
	width, height, err := term.GetSize(int(terminalSize.Fd()))
	if err != nil {
		return nil, fmt.Errorf("get terminal size: %w", err)
	}

	state, err := term.MakeRaw(inputFD)
	if err != nil {
		return nil, fmt.Errorf("put terminal in raw mode: %w", err)
	}
	restore := func() {
		_ = term.Restore(inputFD, state)
	}

	if err := session.RequestPty("xterm", height, width, nil); err != nil {
		restore()
		return nil, fmt.Errorf("request SSH pseudo-terminal: %w", err)
	}

	resizeSignals := make(chan os.Signal, 1)
	stopSignals := startWindowSizeNotifications(terminalSize, width, height, resizeSignals)
	resizeCtx, stopResize := context.WithCancel(ctx)
	go forwardSSHWindowChanges(resizeCtx, terminalSize, session, resizeSignals)

	return func() {
		stopResize()
		stopSignals()
		restore()
	}, nil
}

func sshPTYAvailable(terminalInput, terminalSize *os.File, isTerminal func(int) bool) bool {
	return terminalInput != nil && terminalSize != nil &&
		isTerminal(int(terminalInput.Fd())) && isTerminal(int(terminalSize.Fd()))
}

func forwardSSHWindowChanges(ctx context.Context, terminal *os.File, session *ssh.Session, signals <-chan os.Signal) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			width, height, err := term.GetSize(int(terminal.Fd()))
			if err == nil {
				_ = session.WindowChange(height, width)
			}
		}
	}
}
