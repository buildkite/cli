package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/pkg/browser"
	"namespacelabs.dev/integrations/api"
	namespaceauth "namespacelabs.dev/integrations/auth"
	"namespacelabs.dev/integrations/network/netcopy"
	"namespacelabs.dev/integrations/nsc/ingress"
)

const (
	getKubernetesClusterMethod = "namespace.private.vm.GlobalVMService/GetKubernetesCluster"
	// This matches the nsc API contract that exposes ServiceState credentials.
	namespaceAPIVersion = "160"
)

type VNCCmd struct {
	InstanceID string `arg:"" name:"instance-id" help:"Namespace instance ID" required:""`
}

func (c *VNCCmd) Help() string {
	return `
Examples:
  # Connect a local VNC client to a Namespace instance
  $ bk job vnc i_2d4f7b8c9a
`
}

func (c *VNCCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return c.run(ctx, kongCtx.Stdout, globals.IsQuiet(), defaultVNCDependencies())
}

type vncCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type vncService struct {
	Name        string          `json:"name,omitempty"`
	Status      string          `json:"status,omitempty"`
	Endpoint    string          `json:"endpoint,omitempty"`
	Credentials *vncCredentials `json:"credentials,omitempty"`
}

type vncDependencies struct {
	loadToken    func() (api.TokenSource, error)
	getService   func(context.Context, *http.Client, api.TokenSource, string) (*vncService, error)
	dialEndpoint func(context.Context, api.TokenSource, string) (net.Conn, error)
	listen       func(context.Context, string, string) (net.Listener, error)
	openURL      func(string) error
	proxy        func(net.Conn, net.Conn) error
	httpClient   *http.Client
}

func defaultVNCDependencies() vncDependencies {
	return vncDependencies{
		loadToken:  namespaceauth.LoadDefaults,
		getService: getVNCService,
		dialEndpoint: func(ctx context.Context, token api.TokenSource, endpoint string) (net.Conn, error) {
			return ingress.DialEndpoint(ctx, io.Discard, token, endpoint)
		},
		listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			return new(net.ListenConfig).Listen(ctx, network, address)
		},
		openURL: browser.OpenURL,
		proxy: func(local, remote net.Conn) error {
			return netcopy.CopyConns(nil, local, remote)
		},
		httpClient: http.DefaultClient,
	}
}

func (c *VNCCmd) run(ctx context.Context, stdout io.Writer, quiet bool, deps vncDependencies) error {
	token, err := deps.loadToken()
	if err != nil {
		return fmt.Errorf("load Namespace credentials: %w", err)
	}

	service, err := deps.getService(ctx, deps.httpClient, token, c.InstanceID)
	if err != nil {
		return err
	}

	remote, err := deps.dialEndpoint(ctx, token, service.Endpoint)
	if err != nil {
		return fmt.Errorf("connect to the Namespace VNC service: %w", err)
	}
	defer remote.Close()

	writeVNCStatus(stdout, quiet, "Connected to instance.")

	listener, err := deps.listen(ctx, "tcp", "127.0.0.1:0")
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
		proxyEvents <- proxyEvent{err: deps.proxy(local, remote)}
	}()

	stopCleanup := context.AfterFunc(ctx, func() {
		_ = listener.Close()
		_ = remote.Close()
	})
	defer stopCleanup()

	writeVNCStatus(stdout, quiet, "Opening VNC client...")
	if err := deps.openURL(vncClientURL(listener.Addr().String(), service.Credentials)); err != nil {
		return fmt.Errorf("open the local VNC client: %w", err)
	}

	event := <-proxyEvents
	if event.connected {
		writeVNCStatus(stdout, quiet, "Client connected.")
		event = <-proxyEvents
		writeVNCStatus(stdout, quiet, "Client disconnected, leaving.")
	}

	if event.err != nil {
		if ctx.Err() != nil || errors.Is(event.err, net.ErrClosed) {
			return nil
		}
		return fmt.Errorf("proxy the VNC connection: %w", event.err)
	}

	return nil
}

func writeVNCStatus(w io.Writer, quiet bool, message string) {
	if !quiet {
		fmt.Fprintln(w, message)
	}
}

func vncClientURL(address string, credentials *vncCredentials) string {
	username, password := "admin", "admin"
	if credentials != nil {
		username, password = credentials.Username, credentials.Password
	}

	return (&url.URL{
		Scheme: "vnc",
		User:   url.UserPassword(username, password),
		Host:   address,
	}).String()
}

func getVNCService(ctx context.Context, client *http.Client, token api.TokenSource, instanceID string) (*vncService, error) {
	bearer, err := token.IssueToken(ctx, 15*time.Minute, false)
	if err != nil {
		return nil, fmt.Errorf("issue Namespace token: %w", err)
	}

	endpoint, err := namespaceComputeEndpoint(bearer)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(struct {
		ClusterID string `json:"cluster_id"`
	}{ClusterID: instanceID})
	if err != nil {
		return nil, fmt.Errorf("encode Namespace instance request: %w", err)
	}

	requestURL := strings.TrimRight(endpoint, "/") + "/" + getKubernetesClusterMethod
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Namespace instance request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("NS-Internal-Version", namespaceAPIVersion)
	req.Header.Set("User-Agent", "bk")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Namespace instance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var rpcStatus struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&rpcStatus)
		if rpcStatus.Message != "" {
			return nil, fmt.Errorf("fetch Namespace instance: %s: %s", resp.Status, rpcStatus.Message)
		}
		return nil, fmt.Errorf("fetch Namespace instance: %s", resp.Status)
	}

	var response struct {
		Cluster *struct {
			Services []*vncService `json:"service_state,omitempty"`
		} `json:"cluster,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode Namespace instance: %w", err)
	}
	if response.Cluster == nil {
		return nil, fmt.Errorf("namespace instance %q was not returned", instanceID)
	}

	for _, service := range response.Cluster.Services {
		if service == nil || service.Name != "vnc" {
			continue
		}
		if service.Status != "READY" {
			return nil, fmt.Errorf("VNC service for Namespace instance %q is not ready (status: %s)", instanceID, service.Status)
		}
		if service.Endpoint == "" {
			return nil, fmt.Errorf("VNC service for Namespace instance %q has no endpoint", instanceID)
		}
		return service, nil
	}

	return nil, fmt.Errorf("namespace instance %q does not have a VNC service", instanceID)
}

func namespaceComputeEndpoint(bearer string) (string, error) {
	if endpoint := os.Getenv("NSC_ENDPOINT"); endpoint != "" {
		return endpoint, nil
	}

	claims, err := namespaceauth.ExtractClaims(bearer)
	if err != nil {
		return "", fmt.Errorf("determine Namespace compute region: %w", err)
	}

	region := claims.WorkloadRegion
	if region == "" {
		region = claims.PrimaryRegion
	}
	if region == "" {
		region = "eu"
	}

	return fmt.Sprintf("https://%s.compute.namespaceapis.com", region), nil
}
