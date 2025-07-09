package kong

import (
	"context"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

// CLI represents the complete CLI structure for Kong
type CLI struct {
	Version kong.VersionFlag `short:"v" help:"Print the version number"`
	Verbose bool             `short:"V" help:"Enable verbose error output"`

	// Commands
	Agent     AgentCmd     `cmd:"" help:"Manage agents"`
	API       APICmd       `cmd:"" help:"Interact with the Buildkite API"`
	Artifacts ArtifactsCmd `cmd:"" help:"Manage pipeline build artifacts"`
	Build     BuildCmd     `cmd:"" help:"Manage pipeline builds"`
	Cluster   ClusterCmd   `cmd:"" help:"Manage organization clusters"`
	Configure ConfigureCmd `cmd:"" help:"Configure Buildkite API token"`
	Init      InitCmd      `cmd:"" help:"Initialize a pipeline.yaml file"`
	Job       JobCmd       `cmd:"" help:"Manage jobs within a build"`
	Package   PackageCmd   `cmd:"" help:"Manage packages"`
	Pipeline  PipelineCmd  `cmd:"" help:"Manage pipelines"`
	Prompt    PromptCmd    `cmd:"" help:"Shell prompt integration"`
	Use       UseCmd       `cmd:"" help:"Select an organization"`
	User      UserCmd      `cmd:"" help:"Invite users to the organization"`
	Ver       VersionCmd   `cmd:"" name:"version" help:"Print the version of the CLI being used"`
	Docs      DocsCmd      `cmd:"" help:"Generate documentation"`
}

// AgentCmd represents the agent command group
type AgentCmd struct {
	List AgentListCmd `cmd:"" help:"List agents"`
	Stop AgentStopCmd `cmd:"" help:"Stop agents"`
	View AgentViewCmd `cmd:"" help:"View agent details"`
}

type AgentListCmd struct {
	Name         string `help:"Filter by agent name"`
	AgentVersion string `help:"Filter by agent version" name:"agent-version"`
	Hostname     string `help:"Filter by hostname"`
	PerPage      int    `help:"Number of results per page"`
}

type AgentStopCmd struct {
	Force bool `help:"Force stop agents"`
	Limit int  `short:"l" help:"Limit number of agents to stop"`
}

type AgentViewCmd struct {
	Web bool `short:"w" help:"Open in web browser"`
}

// APICmd represents the API command
type APICmd struct {
	Method    string   `short:"X" help:"HTTP method"`
	Header    []string `short:"H" help:"HTTP headers"`
	Data      string   `short:"d" help:"Request data"`
	Analytics bool     `help:"Enable analytics"`
	File      string   `short:"f" help:"Input file"`
}

// ArtifactsCmd represents the artifacts command group
type ArtifactsCmd struct {
	List     ArtifactsListCmd     `cmd:"" help:"List artifacts"`
	Download ArtifactsDownloadCmd `cmd:"" help:"Download artifacts"`
}

type ArtifactsListCmd struct {
	Job      string `short:"j" help:"Filter by job"`
	Pipeline string `short:"p" help:"Filter by pipeline"`
}

type ArtifactsDownloadCmd struct {
	// Download-specific flags would go here
}

// BuildCmd represents the build command group
type BuildCmd struct {
	Cancel   BuildCancelCmd   `cmd:"" help:"Cancel builds"`
	Download BuildDownloadCmd `cmd:"" help:"Download builds"`
	New      BuildNewCmd      `cmd:"" help:"Create new builds"`
	Rebuild  BuildRebuildCmd  `cmd:"" help:"Rebuild builds"`
	View     BuildViewCmd     `cmd:"" help:"View builds"`
	Watch    BuildWatchCmd    `cmd:"" help:"Watch builds"`
}

type BuildCancelCmd struct {
	Web      bool   `short:"w" help:"Open in web browser"`
	Pipeline string `short:"p" help:"Pipeline slug"`
	Yes      bool   `short:"y" help:"Skip confirmation"`
}

type BuildDownloadCmd struct {
	Mine     bool   `short:"m" help:"Filter to current user"`
	Branch   string `short:"b" help:"Filter by branch"`
	User     string `short:"u" help:"Filter by user"`
	Pipeline string `short:"p" help:"Filter by pipeline"`
}

type BuildNewCmd struct {
	Message             string            `short:"m" help:"Commit message"`
	Commit              string            `short:"c" help:"Commit SHA"`
	Branch              string            `short:"b" help:"Branch name"`
	Web                 bool              `short:"w" help:"Open in web browser"`
	Pipeline            string            `short:"p" help:"Pipeline slug"`
	Env                 map[string]string `short:"e" help:"Environment variables"`
	Metadata            map[string]string `short:"M" help:"Build metadata"`
	IgnoreBranchFilters bool              `short:"i" help:"Ignore branch filters"`
	Yes                 bool              `short:"y" help:"Skip confirmation"`
	EnvFile             string            `short:"f" help:"Environment file"`
}

type BuildRebuildCmd struct {
	Mine     bool   `short:"m" help:"Filter to current user"`
	Web      bool   `short:"w" help:"Open in web browser"`
	Branch   string `short:"b" help:"Filter by branch"`
	User     string `short:"u" help:"Filter by user"`
	Pipeline string `short:"p" help:"Filter by pipeline"`
}

type BuildViewCmd struct {
	Mine     bool   `help:"Filter to current user"`
	Web      bool   `help:"Open in web browser"`
	Branch   string `help:"Filter by branch"`
	User     string `help:"Filter by user"`
	Pipeline string `help:"Filter by pipeline"`
}

type BuildWatchCmd struct {
	Pipeline string `short:"p" help:"Pipeline slug"`
	Branch   string `short:"b" help:"Branch name"`
	Interval int    `help:"Polling interval in seconds"`
}

// ClusterCmd represents the cluster command group
type ClusterCmd struct {
	List ClusterListCmd `cmd:"" help:"List clusters"`
	View ClusterViewCmd `cmd:"" help:"View cluster details"`
}

type ClusterListCmd struct{}

type ClusterViewCmd struct{}

// ConfigureCmd represents the configure command group
type ConfigureCmd struct {
	Add ConfigureAddCmd `cmd:"" help:"Add configuration"`
}

type ConfigureAddCmd struct {
	Force bool   `help:"Force overwrite existing configuration"`
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`
}

// InitCmd represents the init command
type InitCmd struct{}

// JobCmd represents the job command group
type JobCmd struct {
	Retry   JobRetryCmd   `cmd:"" help:"Retry jobs"`
	Unblock JobUnblockCmd `cmd:"" help:"Unblock jobs"`
}

type JobRetryCmd struct{}

type JobUnblockCmd struct {
	Data string `help:"Unblock data"`
}

// PackageCmd represents the package command group
type PackageCmd struct {
	Push PackagePushCmd `cmd:"" help:"Push packages"`
}

type PackagePushCmd struct {
	StdinFileName string `short:"n" help:"Stdin file name"`
	Web           bool   `short:"w" help:"Open in web browser"`
}

// PipelineCmd represents the pipeline command group
type PipelineCmd struct {
	Create   PipelineCreateCmd   `cmd:"" help:"Create pipelines"`
	View     PipelineViewCmd     `cmd:"" help:"View pipelines"`
	Validate PipelineValidateCmd `cmd:"" help:"Validate pipelines"`
}

type PipelineCreateCmd struct{}

type PipelineViewCmd struct {
	Web bool `short:"w" help:"Open in web browser"`
}

type PipelineValidateCmd struct {
	File string `short:"f" help:"Pipeline file"`
}

// PromptCmd represents the prompt command
type PromptCmd struct {
	Format string `help:"Output format"`
	Shell  string `help:"Shell type"`
}

// UseCmd represents the use command
type UseCmd struct{}

// UserCmd represents the user command group
type UserCmd struct {
	Invite UserInviteCmd `cmd:"" help:"Invite users"`
}

type UserInviteCmd struct{}

// VersionCmd represents the version command
type VersionCmd struct{}

// DocsCmd represents the new docs command for llms.txt generation
type DocsCmd struct {
	Format string `help:"Output format (llms.txt, markdown, json)" default:"llms.txt"`
	Output string `short:"o" help:"Output file (default: stdout)"`
}

// RunWithFactory creates a Kong parser and runs the CLI
func RunWithFactory(f *factory.Factory, args []string) error {
	cli := &CLI{}

	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": f.Version},
	)

	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}

	// Set up context with factory
	return ctx.Run(context.Background(), f)
}

// GetKongParser returns a Kong parser for the CLI
func GetKongParser(version string) *kong.Kong {
	cli := &CLI{}
	return kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)
}
