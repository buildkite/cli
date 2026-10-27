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
	ClusterName string `help:"Cluster name to create the agent token on (resolved to UUID)" name:"cluster-name" optional:""`
	ClusterUUID string `help:"Cluster UUID to create the agent token on (default: the \"Default\" cluster)" name:"cluster-uuid" optional:""`
	NoToken     bool   `help:"Skip creating an agent token and config file" name:"no-token"`
	ConfigPath  string `help:"Path to write the agent config file" type:"path"`
	BuildPath   string `help:"Directory the agent runs builds in (default: /var/lib/buildkite-agent/builds for configs under /etc/buildkite-agent on Linux, otherwise the platform default)"`
	Force       bool   `help:"Overwrite an existing agent config file"`
}

func (i *InstallCmd) Validate() error {
	if i.ClusterUUID != "" && i.ClusterName != "" {
		return fmt.Errorf("only one of --cluster-uuid or --cluster-name can be specified")
	}
	return nil
}

func (i *InstallCmd) Help() string {
	return `Install the buildkite-agent binary locally.

By default, this also creates an agent token on the Default cluster and writes
a minimal config file so the agent is ready to start.
Use --cluster-name or --cluster-uuid to select a different cluster. If no Default
cluster is accessible, the command lists available clusters and exits.

Examples:
  # Install the latest version of the agent
  $ bk agent install

  # Install a specific version
  $ bk agent install --version "3.112.0"

  # Install to a custom location
  $ bk agent install --dest ~/.local/bin

  # Install using a cluster selected by name
  $ bk agent install --cluster-name "My cluster"

  # Install using a cluster selected by UUID
  $ bk agent install --cluster-uuid "01234567-89ab-cdef-0123-456789abcdef"

  # Install without creating a token/config
  $ bk agent install --no-token

  # Choose where builds run
  $ bk agent install --build-path /data/buildkite/builds

  # Replace an existing config file with a new token
  $ bk agent install --force

An existing config file is never overwritten unless --force is given;
the token in the replaced config is not revoked.
`
}

func (i *InstallCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	configPath := i.ConfigPath
	if configPath == "" {
		configPath = bkAgent.DefaultConfigPath(userOS)
	}

	var f *factory.Factory
	var clusterID string
	if !i.NoToken {
		// Check before downloading or creating a token, so a refusal has no side effects
		if err := checkConfigPath(configPath, i.Force); err != nil {
			return err
		}
		var err error
		f, err = factory.New(factory.WithDebug(globals.EnableDebug()))
		if err != nil {
			return fmt.Errorf("initializing API client: %w", err)
		}
		clusterID, err = bkAgent.FindCluster(context.Background(), f, f.Config.OrganizationSlug(), i.ClusterUUID, i.ClusterName)
		if err != nil {
			return fmt.Errorf("finding cluster: %w", err)
		}
	}

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
		if err := i.createTokenAndConfig(f, clusterID, configPath); err != nil {
			return err
		}
	}

	return nil
}

// checkConfigPath refuses to replace an existing config file unless force is set,
// because WriteAgentConfig rewrites the whole file and would drop any other settings.
func checkConfigPath(path string, force bool) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking agent config: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("agent config path %s is a directory", path)
	}
	if force {
		return nil
	}
	return fmt.Errorf("agent config already exists at %s; use --force to replace it (other settings in the file will be lost), or --no-token to only install the binary", path)
}

func (i *InstallCmd) createTokenAndConfig(f *factory.Factory, clusterID, configPath string) error {
	ctx := context.Background()
	org := f.Config.OrganizationSlug()

	fmt.Println("Creating agent token...")
	token, err := bkAgent.CreateAgentToken(ctx, f, org, clusterID, "Token created by bk agent install")
	if err != nil {
		return fmt.Errorf("creating agent token: %w", err)
	}

	// BuildPath isn't type:"path" because Kong maps an empty value to the working
	// directory, which would silently become the build path.
	explicitBuildPath := i.BuildPath
	if explicitBuildPath != "" {
		explicitBuildPath = kong.ExpandPath(explicitBuildPath)
	}
	buildPath := bkAgent.ResolveBuildPath(userOS, configPath, explicitBuildPath)
	if err := bkAgent.WriteAgentConfig(configPath, token.Token, buildPath, nil); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

	fmt.Printf("Agent config written to %s\n", configPath)
	fmt.Printf("Builds will run in %s\n", buildPath)

	// Under sudo the home-based default build path usually resolves to root's home,
	// which an agent running as any other user (such as the packaged buildkite-agent
	// service) can't write to.
	if userOS == "linux" && i.BuildPath == "" && !bkAgent.IsSystemConfigPath(userOS, configPath) && os.Getenv("SUDO_USER") != "" {
		fmt.Println()
		fmt.Println("Warning: running under sudo. If the agent will run as another user, it may not")
		fmt.Printf("  be able to write to %s. Change build-path in %s if so.\n", buildPath, configPath)
	}
	return nil
}
