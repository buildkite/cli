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

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"

	bkAgent "github.com/buildkite/cli/v3/internal/agent"
)

// RunCmd spins up an ephemeral buildkite-agent attached to a cluster.
type RunCmd struct {
	Version     string `help:"Specify an agent version to run" default:"latest"`
	ClusterUUID string `help:"Cluster UUID to create the agent token on (default: the \"Default\" cluster)" name:"cluster-uuid" optional:""`
	Queue       string `help:"Queue for the agent to listen on" default:"default"`
}

func (r *RunCmd) Help() string {
	return `Run an ephemeral buildkite-agent locally.

Downloads the agent binary, creates a cluster token, and starts the agent.
All temporary files are cleaned up when the agent is stopped with Ctrl+C.

Examples:
  # Run the latest agent on the Default cluster
  $ bk agent run

  # Run a specific version
  $ bk agent run --version "3.112.0"

  # Run on a specific cluster
  $ bk agent run --cluster-uuid "01234567-89ab-cdef-0123-456789abcdef"

  # Run on a specific queue
  $ bk agent run --queue "deploy"
`
}

func (r *RunCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	// Create API client and provision a cluster token
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return fmt.Errorf("initializing API client: %w", err)
	}

	ctx := context.Background()
	org := f.Config.OrganizationSlug()

	clusterID, err := bkAgent.FindCluster(ctx, f, org, r.ClusterUUID)
	if err != nil {
		return fmt.Errorf("finding cluster: %w", err)
	}

	fmt.Println("Creating agent token...")
	token, err := bkAgent.CreateAgentToken(ctx, f, org, clusterID, "Ephemeral token created by bk agent run")
	if err != nil {
		return fmt.Errorf("creating agent token: %w", err)
	}

	// Write a temporary config file
	configPath := filepath.Join(tmpDir, "buildkite-agent.cfg")
	buildPath := filepath.Join(tmpDir, "builds")
	var tags []string
	if r.Queue != "" {
		tags = append(tags, "queue="+r.Queue)
	}
	if err := bkAgent.WriteAgentConfig(configPath, token, buildPath, tags); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

	// Catch signals so we wait for the agent to shut down gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Starting buildkite-agent v%s...\n", version)
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

	select {
	case <-sigCh:
		fmt.Println("\nShutting down agent...")
		// The agent already received the signal (same process group)
		// and will finish any running job before exiting.
		<-errCh
		fmt.Println("Agent stopped.")
		return nil

	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("agent exited with error: %w", err)
		}
		return nil
	}
}
