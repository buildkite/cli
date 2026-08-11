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
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/gorilla/websocket"
	"github.com/pkg/browser"
	"namespacelabs.dev/integrations/network/netcopy"
	"namespacelabs.dev/integrations/nsc/ingress"
)

type VNCCmd struct {
	JobID string `arg:"" name:"job-uuid" help:"UUID of the hosted agent job" required:""`
}

type vncSession struct {
	Endpoint    string                `json:"endpoint"`
	AccessToken string                `json:"access_token"`
	ExpiresAt   time.Time             `json:"expires_at"`
	VNC         vncSessionCredentials `json:"vnc"`
}

type vncSessionCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s vncSession) validate() error {
	switch {
	case s.Endpoint == "":
		return errors.New("buildkite API returned a VNC session without an endpoint")
	case s.AccessToken == "":
		return errors.New("buildkite API returned a VNC session without an access token")
	case s.ExpiresAt.IsZero():
		return errors.New("buildkite API returned a VNC session without an expiry")
	case s.VNC.Username == "":
		return errors.New("buildkite API returned a VNC session without a VNC username")
	case s.VNC.Password == "":
		return errors.New("buildkite API returned a VNC session without a VNC password")
	default:
		return nil
	}
}

func (c *VNCCmd) Help() string {
	return `
Examples:
	# Connect a local VNC client to a running hosted macOS job
	$ bk job vnc 0190046e-e199-453b-a302-a21a4d649d31
`
}

func (c *VNCCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	return c.run(ctx, kongCtx.Stdout, globals.IsQuiet(), f.RestAPIClient, organization, browser.OpenURL)
}

func (c *VNCCmd) run(ctx context.Context, stdout io.Writer, quiet bool, client *buildkite.Client, organization string, openURL func(string) error) error {
	session, err := createVNCSession(ctx, client, organization, c.JobID)
	if err != nil {
		return fmt.Errorf("create VNC session: %w", err)
	}

	remote, err := ingress.DialEndpoint(ctx, io.Discard, vncAccessToken(session.AccessToken), session.Endpoint)
	if err != nil {
		return fmt.Errorf("connect to the VNC service: %w", err)
	}
	defer remote.Close()

	writeVNCStatus(stdout, quiet, "Connected to job.")

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for the local VNC client: %w", err)
	}
	defer listener.Close()

	type proxyEvent struct {
		connected bool
		err       error
	}
	proxyEvents := make(chan proxyEvent, 2)
	go func() {
		local, acceptErr := listener.Accept()
		if acceptErr != nil {
			proxyEvents <- proxyEvent{err: acceptErr}
			return
		}
		_ = listener.Close()
		defer local.Close()

		proxyEvents <- proxyEvent{connected: true}
		proxyEvents <- proxyEvent{err: netcopy.CopyConns(nil, local, remote)}
	}()

	stopCleanup := context.AfterFunc(ctx, func() {
		_ = listener.Close()
		_ = remote.Close()
	})
	defer stopCleanup()

	writeVNCStatus(stdout, quiet, "Opening VNC client...")
	if err := openURL(vncClientURL(listener.Addr().String(), session.VNC.Username, session.VNC.Password)); err != nil {
		return fmt.Errorf("open the local VNC client: %w", err)
	}

	event := <-proxyEvents
	if event.connected {
		writeVNCStatus(stdout, quiet, "Client connected.")
		event = <-proxyEvents
		writeVNCStatus(stdout, quiet, "Client disconnected, leaving.")
	}

	if event.err != nil {
		if isExpectedVNCDisconnect(ctx, event.err) {
			return nil
		}
		return fmt.Errorf("proxy the VNC connection: %w", event.err)
	}

	return nil
}

func isExpectedVNCDisconnect(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
		return true
	}

	var closeErr *websocket.CloseError
	return errors.As(err, &closeErr) &&
		(closeErr.Code == websocket.CloseNormalClosure || closeErr.Code == websocket.CloseGoingAway)
}

func writeVNCStatus(w io.Writer, quiet bool, message string) {
	if !quiet {
		fmt.Fprintln(w, message)
	}
}

func vncClientURL(address, username, password string) string {
	return (&url.URL{
		Scheme: "vnc",
		User:   url.UserPassword(username, password),
		Host:   address,
	}).String()
}

type vncAccessToken string

func (t vncAccessToken) IssueToken(context.Context, time.Duration, bool) (string, error) {
	return string(t), nil
}
