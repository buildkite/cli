package job

import (
	"context"
	"errors"
	"fmt"
	"io"
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

	remote, err := ingress.DialEndpoint(ctx, io.Discard, remoteAccessToken(session.AccessToken), session.Endpoint)
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
