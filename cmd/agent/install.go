package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"

	bkAgent "github.com/buildkite/cli/v3/internal/agent"
)

var (
	userArch = runtime.GOARCH
	userOS   = runtime.GOOS
)

// InstallCmd allows users to define which agent version they want to install
// We will take care of OS/arch in the command itself
type InstallCmd struct {
	Version     string `help:"Specify an agent version to install" default:"latest"`
	Dest        string `help:"Destination directory for the binary" type:"path"`
	ClusterUUID string `help:"Cluster UUID to create the agent token on (default: the \"Default\" cluster)" name:"cluster-uuid" optional:""`
	NoToken     bool   `help:"Skip creating an agent token and config file" name:"no-token"`
	ConfigPath  string `help:"Path to write the agent config file" type:"path"`
}

func (i *InstallCmd) Help() string {
	return `Install the buildkite-agent binary locally.

By default, this also creates an agent token on the Default cluster and writes
a minimal config file so the agent is ready to start.

Examples:
  # Install the latest version of the agent
  $ bk agent install

  # Install a specific version
  $ bk agent install --version "3.112.0"

  # Install to a custom location
  $ bk agent install --dest ~/.local/bin

  # Install without creating a token/config
  $ bk agent install --no-token
`
}

func (i *InstallCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	dest := i.Dest
	if dest == "" {
		dest = bkAgent.DefaultBinDir(userOS)
	}

	// Check for existing installations in PATH
	if existing := bkAgent.FindExisting(userOS); existing != nil {
		destBinary := filepath.Join(dest, bkAgent.BinaryName(userOS))
		if existing.Path != destBinary {
			fmt.Printf("Warning: existing buildkite-agent found at %s", existing.Path)
			if existing.Version != "" {
				fmt.Printf(" (%s)", existing.Version)
			}
			fmt.Println()
			fmt.Printf("  The new install at %s may be shadowed in your PATH.\n", destBinary)
			fmt.Println()
		}
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	version := i.Version
	if version == "latest" {
		resolved, err := bkAgent.ResolveLatestVersion()
		if err != nil {
			return fmt.Errorf("resolving latest version: %w", err)
		}
		version = resolved
	}

	version = strings.TrimPrefix(version, "v")

	downloadURL := bkAgent.BuildDownloadURL(version, userOS, userArch)
	fmt.Printf("Downloading buildkite-agent v%s for %s/%s...\n", version, userOS, userArch)

	tmpFile, err := bkAgent.DownloadToTemp(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading agent: %w", err)
	}
	defer os.Remove(tmpFile)

	// Verify the download checksum
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

	if err := bkAgent.ExtractBinary(tmpFile, dest, userOS); err != nil {
		return fmt.Errorf("extracting agent: %w", err)
	}

	binaryName := bkAgent.BinaryName(userOS)
	fmt.Printf("Installed buildkite-agent to %s\n", filepath.Join(dest, binaryName))

	if !i.NoToken {
		if err := i.createTokenAndConfig(globals); err != nil {
			return err
		}
	}

	return nil
}

func (i *InstallCmd) createTokenAndConfig(globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return fmt.Errorf("initializing API client: %w", err)
	}

	ctx := context.Background()
	org := f.Config.OrganizationSlug()

	clusterID, err := bkAgent.FindCluster(ctx, f, org, i.ClusterUUID)
	if err != nil {
		return fmt.Errorf("finding default cluster: %w", err)
	}

	fmt.Println("Creating agent token...")
	token, err := bkAgent.CreateAgentToken(ctx, f, org, clusterID, "Token created by bk agent install")
	if err != nil {
		return fmt.Errorf("creating agent token: %w", err)
	}

	configPath := i.ConfigPath
	if configPath == "" {
		configPath = bkAgent.DefaultConfigPath(userOS)
	}

	buildPath := bkAgent.DefaultBuildPath(userOS)
	if err := bkAgent.WriteAgentConfig(configPath, token, buildPath, nil); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

	fmt.Printf("Agent config written to %s\n", configPath)
	return nil
}
