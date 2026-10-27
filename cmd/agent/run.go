package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"

	bkAgent "github.com/buildkite/cli/v3/internal/agent"
)

// RunCmd spins up an ephemeral buildkite-agent attached to a cluster.
type RunCmd struct {
	Version     string `help:"Specify an agent version to run" default:"latest"`
	ClusterName string `help:"Cluster name to create the agent token on (resolved to UUID)" name:"cluster-name" optional:""`
	ClusterUUID string `help:"Cluster UUID to create the agent token on (default: the \"Default\" cluster)" name:"cluster-uuid" optional:""`
	Queue       string `help:"Queue for the agent to listen on" default:"default"`
}

func (r *RunCmd) Validate() error {
	if r.ClusterUUID != "" && r.ClusterName != "" {
		return fmt.Errorf("only one of --cluster-uuid or --cluster-name can be specified")
	}
	return nil
}

func (r *RunCmd) Help() string {
	return `Run an ephemeral buildkite-agent locally.

Downloads the agent binary, creates a cluster agent token, and starts the agent.
When the agent stops, the token is revoked and temporary files are removed.
Without a cluster selector, uses the cluster named exactly Default. If it is not
accessible, lists available clusters and exits. Use --cluster-name or --cluster-uuid
to select a different cluster.

Examples:
  # Run the latest agent on the Default cluster
  $ bk agent run

  # Run a specific version
  $ bk agent run --version "3.112.0"

  # Run on a specific cluster by name
  $ bk agent run --cluster-name "My cluster"

  # Run on a specific cluster by UUID
  $ bk agent run --cluster-uuid "01234567-89ab-cdef-0123-456789abcdef"

  # Run on a specific queue
  $ bk agent run --queue "deploy"
`
}

func (r *RunCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	// Ctrl+C also stops a reader such as `bk agent run | tee`. Go kills a
	// program that writes to a closed stdout unless it asks for SIGPIPE, which
	// would skip revoking the token and removing temporary files. Ask for it
	// for the rest of the run so the write fails instead. Unlike
	// signal.Ignore, this leaves SIGPIPE at its default in the agent.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)

	// Resolve the cluster before downloading the agent or creating a token.
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return fmt.Errorf("initializing API client: %w", err)
	}

	ctx := context.Background()
	org := f.Config.OrganizationSlug()

	clusterID, err := bkAgent.FindCluster(ctx, f, org, r.ClusterUUID, r.ClusterName)
	if err != nil {
		return fmt.Errorf("finding cluster: %w", err)
	}

	// Track temp directory for cleanup
	tmpDir, err := os.MkdirTemp("", "bk-agent-run-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		fmt.Println("Cleaning up temporary files...")
		os.RemoveAll(tmpDir)
	}()

	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH

	version := r.Version
	if version == "latest" {
		resolved, err := bkAgent.ResolveLatestVersion()
		if err != nil {
			return fmt.Errorf("resolving latest version: %w", err)
		}
		version = resolved
	}
	version = strings.TrimPrefix(version, "v")

	downloadURL := bkAgent.BuildDownloadURL(version, targetOS, targetArch)
	fmt.Printf("Downloading buildkite-agent v%s for %s/%s...\n", version, targetOS, targetArch)

	tmpFile, err := bkAgent.DownloadToTemp(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading agent: %w", err)
	}
	defer os.Remove(tmpFile)

	fmt.Println("Verifying checksum...")
	sumsURL := bkAgent.BuildSHA256SumsURL(version)
	archiveFilename := filepath.Base(downloadURL)
	expectedHash, err := bkAgent.FetchExpectedSHA256(sumsURL, archiveFilename)
	if err != nil {
		return fmt.Errorf("fetching checksum: %w", err)
	}
	if err := bkAgent.VerifySHA256(tmpFile, expectedHash); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := bkAgent.ExtractBinary(tmpFile, tmpDir, targetOS); err != nil {
		return fmt.Errorf("extracting agent: %w", err)
	}

	binaryPath := filepath.Join(tmpDir, bkAgent.BinaryName(targetOS))

	// Catch signals before the token exists, so stopping bk always revokes it.
	// Leave SIGHUP and SIGQUIT ignored if bk was started that way, such as
	// SIGHUP under nohup.
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	for _, sig := range []os.Signal{syscall.SIGHUP, syscall.SIGQUIT} {
		if !signal.Ignored(sig) {
			signals = append(signals, sig)
		}
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)
	defer signal.Stop(sigCh)

	return runAgent(ctx, f, org, clusterID, binaryPath, tmpDir, r.Queue, sigCh)
}

// runAgent creates a cluster agent token, runs the agent with it until the
// agent exits, and revokes the token before returning.
func runAgent(ctx context.Context, f *factory.Factory, org, clusterID, binaryPath, dir, queue string, sigCh <-chan os.Signal) error {
	fmt.Println("Creating agent token...")
	token, err := bkAgent.CreateAgentToken(ctx, f, org, clusterID, "Ephemeral token created by bk agent run")
	if err != nil {
		return fmt.Errorf("creating agent token: %w", err)
	}
	defer revokeAgentToken(f, org, clusterID, token.ID)

	// Write a temporary config file
	configPath := filepath.Join(dir, "buildkite-agent.cfg")
	buildPath := filepath.Join(dir, "builds")
	var tags []string
	if queue != "" {
		tags = append(tags, "queue="+queue)
	}
	if err := bkAgent.WriteAgentConfig(configPath, token.Token, buildPath, tags); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

	// A signal before the agent starts never reaches it, so stop here instead.
	select {
	case <-sigCh:
		return nil
	default:
	}

	fmt.Println("Starting buildkite-agent...")
	cmd := exec.Command(binaryPath, "start", "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	// Wait for the agent to exit in the background
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()

	shuttingDown, hungUp := false, false
	for {
		select {
		case sig := <-sigCh:
			if !shuttingDown {
				shuttingDown = true
				fmt.Println("\nShutting down agent...")
			}
			// The terminal sends Ctrl+C and Ctrl+\ to the agent too, as it is in
			// the same process group. SIGTERM is usually sent to bk alone, so
			// pass each one on: a second SIGTERM cancels the running job, as it
			// would for the agent. Closing the terminal sends SIGHUP more than
			// once, but should only ask the agent to stop gracefully.
			switch {
			case sig == syscall.SIGTERM:
				_ = cmd.Process.Signal(syscall.SIGTERM)
			case sig == syscall.SIGHUP && !hungUp:
				hungUp = true
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}

		case err := <-errCh:
			if shuttingDown {
				fmt.Println("Agent stopped.")
				return nil
			}
			if err != nil {
				return fmt.Errorf("agent exited with error: %w", err)
			}
			return nil
		}
	}
}

// revokeAgentToken revokes the token created for this run. The timeout stops
// an unresponsive API from keeping bk running after the agent has stopped.
func revokeAgentToken(f *factory.Factory, org, clusterID, tokenID string) {
	fmt.Println("Revoking agent token...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := f.RestAPIClient.ClusterTokens.Delete(ctx, org, clusterID, tokenID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not revoke agent token %s: %v\n", tokenID, err)
		fmt.Fprintf(os.Stderr, "Revoke it with: bk --org %s api --method DELETE /clusters/%s/tokens/%s\n", org, clusterID, tokenID)
	}
}
