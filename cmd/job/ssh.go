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
	SSH sshSessionCredentials `json:"ssh"`
}

type sshSessionCredentials struct {
	Username   string `json:"username"`
	PrivateKey string `json:"private_key"`
}

func (s sshSession) validate() error {
	if err := s.remoteSession.validate("an SSH"); err != nil {
		return err
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

	if s.SSH.Username == "" {
		return errors.New("buildkite API returned an SSH session without an SSH username")
	}
	if s.SSH.PrivateKey == "" {
		return errors.New("buildkite API returned an SSH session without an SSH private key")
	}

	return nil
}

func (c *SSHCmd) Help() string {
	return `
Examples:
	# Open an interactive shell on a running hosted macOS job
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
	return c.runWithDialer(ctx, streams, client, organization, dialSSHEndpoint)
}

type sshEndpointDialer func(context.Context, remoteAccessToken, string) (net.Conn, error)

func dialSSHEndpoint(ctx context.Context, token remoteAccessToken, endpoint string) (net.Conn, error) {
	return ingress.DialEndpoint(ctx, io.Discard, token, endpoint)
}

func (c *SSHCmd) runWithDialer(ctx context.Context, streams sshStreams, client *buildkite.Client, organization string, dialEndpoint sshEndpointDialer) error {
	session, err := createSSHSession(ctx, client, organization, c.JobID)
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(session.SSH.PrivateKey))
	if err != nil {
		return fmt.Errorf("parse SSH private key: %w", err)
	}

	remote, err := dialEndpoint(ctx, remoteAccessToken(session.AccessToken), session.Endpoint)
	if err != nil {
		return fmt.Errorf("connect to the SSH service: %w", err)
	}
	defer remote.Close()
	stopRemoteClose := context.AfterFunc(ctx, func() {
		_ = remote.Close()
	})
	defer stopRemoteClose()

	config := &ssh.ClientConfig{
		User: session.SSH.Username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// Namespace provides an ephemeral SSH server behind an authenticated WSS
		// endpoint, so it has no stable host key to put in known_hosts.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
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

func isExpectedSSHExit(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}

	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
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
