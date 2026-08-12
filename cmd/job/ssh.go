package job

import (
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
	Username   string `json:"username"`
	PrivateKey string `json:"private_key"`
}

func (s sshSession) validate() error {
	if err := s.remoteSession.validate("an SSH"); err != nil {
		return err
	}

	switch s.Transport {
	case "":
		return errors.New("buildkite API returned an SSH session without a transport")
	case sshTransportTCP:
		if _, err := sshTCPEndpointAddress(s.Endpoint); err != nil {
			return fmt.Errorf("buildkite API returned an SSH session with an invalid TCP endpoint: %w", err)
		}
	case sshTransportNamespaceIngress:
		// Namespace ingress validates its own endpoint when dialing.
	default:
		return fmt.Errorf("buildkite API returned unsupported SSH transport %q", s.Transport)
	}

	switch {
	case s.SSH.Username == "":
		return errors.New("buildkite API returned an SSH session without an SSH username")
	case s.SSH.PrivateKey == "":
		return errors.New("buildkite API returned an SSH session without an SSH private key")
	default:
		return nil
	}
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
		Stdin:    os.Stdin,
		Stdout:   kongCtx.Stdout,
		Stderr:   kongCtx.Stderr,
		Terminal: os.Stdin,
	}, f.RestAPIClient, organization)
}

type sshStreams struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Terminal *os.File
}

func (c *SSHCmd) run(ctx context.Context, streams sshStreams, client *buildkite.Client, organization string) error {
	session, err := createSSHSession(ctx, client, organization, c.JobID)
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(session.SSH.PrivateKey))
	if err != nil {
		return fmt.Errorf("parse SSH private key: %w", err)
	}

	remote, err := dialSSHSession(ctx, session)
	if err != nil {
		return fmt.Errorf("connect to the SSH service: %w", err)
	}
	defer remote.Close()

	config := &ssh.ClientConfig{
		User: session.SSH.Username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// Hosted jobs use ephemeral SSH servers, and the session contract does not
		// provide a stable host key that can be verified through known_hosts.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	}

	connection, channels, requests, err := ssh.NewClientConn(remote, remote.RemoteAddr().String(), config)
	if err != nil {
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
		defer remoteStdin.Close()
	}

	restoreTerminal, err := configureSSHPTY(ctx, sshShell, streams.Terminal)
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
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("wait for SSH shell: %w", err)
	}

	return nil
}

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

func configureSSHPTY(ctx context.Context, session *ssh.Session, terminal *os.File) (func(), error) {
	if terminal == nil || !term.IsTerminal(int(terminal.Fd())) {
		return func() {}, nil
	}

	fd := int(terminal.Fd())
	width, height, err := term.GetSize(fd)
	if err != nil {
		return nil, fmt.Errorf("get terminal size: %w", err)
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("put terminal in raw mode: %w", err)
	}
	restore := func() {
		_ = term.Restore(fd, state)
	}

	if err := session.RequestPty("xterm", height, width, nil); err != nil {
		restore()
		return nil, fmt.Errorf("request SSH pseudo-terminal: %w", err)
	}

	resizeSignals := make(chan os.Signal, 1)
	stopSignals := startWindowSizeNotifications(resizeSignals)
	resizeCtx, stopResize := context.WithCancel(ctx)
	go forwardSSHWindowChanges(resizeCtx, terminal, session, resizeSignals)

	return func() {
		stopResize()
		stopSignals()
		restore()
	}, nil
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
