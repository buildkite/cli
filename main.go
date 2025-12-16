package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/cmd/agent"
	"github.com/buildkite/cli/v3/cmd/api"
	"github.com/buildkite/cli/v3/cmd/artifacts"
	"github.com/buildkite/cli/v3/cmd/build"
	"github.com/buildkite/cli/v3/cmd/cluster"
	"github.com/buildkite/cli/v3/cmd/configure"
	bkInit "github.com/buildkite/cli/v3/cmd/init"
	"github.com/buildkite/cli/v3/cmd/job"
	"github.com/buildkite/cli/v3/cmd/organization"
	"github.com/buildkite/cli/v3/cmd/pipeline"
	"github.com/buildkite/cli/v3/cmd/pkg"
	"github.com/buildkite/cli/v3/cmd/use"
	"github.com/buildkite/cli/v3/cmd/user"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/cmd/whoami"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
	"github.com/buildkite/cli/v3/pkg/output"
)

// Kong CLI structure, with base commands defined as additional commands are defined in their respective files
type CLI struct {
	// Global flags
	Yes     bool `help:"Skip all confirmation prompts" short:"y"`
	NoInput bool `help:"Disable all interactive prompts" name:"no-input"`
	Quiet   bool `help:"Suppress progress output" short:"q"`
	// Verbose bool `help:"Enable verbose error output" short:"V"` // TODO: Implement this, atm this is just a skeleton flag

	Agent        AgentCmd               `cmd:"" help:"Manage agents"`
	Api          ApiCmd                 `cmd:"" help:"Interact with the Buildkite API"`
	Artifacts    ArtifactsCmd           `cmd:"" help:"Manage pipeline build artifacts"`
	Build        BuildCmd               `cmd:"" help:"Manage pipeline builds"`
	Cluster      ClusterCmd             `cmd:"" help:"Manage organization clusters"`
	Configure    configure.ConfigureCmd `cmd:"" help:"Configure Buildkite API token"`
	Init         bkInit.InitCmd         `cmd:"" help:"Initialize a pipeline.yaml file"`
	Job          JobCmd                 `cmd:"" help:"Manage jobs within a build"`
	Organization OrganizationCmd        `cmd:"" help:"Manage organizations" aliases:"org"`
	Pipeline     PipelineCmd            `cmd:"" help:"Manage pipelines"`
	Package      PackageCmd             `cmd:"" help:"Manage packages"`
	Use          use.UseCmd             `cmd:"" help:"Select an organization"`
	User         UserCmd                `cmd:"" help:"Invite users to the organization"`
	Version      VersionCmd             `cmd:"" help:"Print the version of the CLI being used"`
	Whoami       whoami.WhoAmICmd       `cmd:"" help:"Print the current user and organization"`
}

// Hybrid delegation commands, we should delete from these when native Kong implementations ready
type (
	VersionCmd struct {
		version.VersionCmd `cmd:"" help:"Print the version of the CLI being used"`
	}
	AgentCmd struct {
		Pause  agent.PauseCmd  `cmd:"" help:"Pause a Buildkite agent."`
		List   agent.ListCmd   `cmd:"" help:"List agents." alias:"ls"`
		Resume agent.ResumeCmd `cmd:"" help:"Resume a Buildkite agent."`
		Stop   agent.StopCmd   `cmd:"" help:"Stop Buildkite agents."`
		View   agent.ViewCmd   `cmd:"" help:"View details of an agent."`
	}
	ArtifactsCmd struct {
		Download artifacts.DownloadCmd `cmd:"" help:"Download an artifact by its UUID."`
		List     artifacts.ListCmd     `cmd:"" help:"List artifacts for a build or a job in a build." aliases:"ls"`
	}
	BuildCmd struct {
		Create   build.CreateCmd   `cmd:"" aliases:"new" help:"Create a new build."` // Aliasing "new" because we've renamed this to "create", but we need to support backwards compatibility
		Cancel   build.CancelCmd   `cmd:"" help:"Cancel a build."`
		View     build.ViewCmd     `cmd:"" help:"View build information."`
		List     build.ListCmd     `cmd:"" help:"List builds." aliases:"ls"`
		Download build.DownloadCmd `cmd:"" help:"Download resources for a build."`
		Rebuild  build.RebuildCmd  `cmd:"" help:"Rebuild a build."`
		Watch    build.WatchCmd    `cmd:"" help:"Watch a build's progress in real-time."`
	}
	ClusterCmd struct {
		List cluster.ListCmd `cmd:"" help:"List clusters." aliases:"ls"`
		View cluster.ViewCmd `cmd:"" help:"View cluster information."`
	}
	JobCmd struct {
		Cancel  job.CancelCmd  `cmd:"" help:"Cancel a job."`
		List    job.ListCmd    `cmd:"" help:"List jobs." aliases:"ls"`
		Retry   job.RetryCmd   `cmd:"" help:"Retry a job."`
		Unblock job.UnblockCmd `cmd:"" help:"Unblock a job."`
	}
	OrganizationCmd struct {
		List organization.ListCmd `cmd:"" help:"List configured organizations." aliases:"ls"`
	}
	PackageCmd struct {
		Push pkg.PushCmd `cmd:"" help:"Push a new package to a Buildkite registry"`
	}
	PipelineCmd struct {
		Create   pipeline.CreateCmd   `cmd:"" help:"Create a new pipeline."`
		List     pipeline.ListCmd     `cmd:"" help:"List pipelines." aliases:"ls"`
		Migrate  pipeline.MigrateCmd  `cmd:"" help:"Migrate a CI/CD pipeline configuration to Buildkite format."`
		Validate pipeline.ValidateCmd `cmd:"" help:"Validate a pipeline YAML file."`
		View     pipeline.ViewCmd     `cmd:"" help:"View a pipeline."`
	}
	UserCmd struct {
		Invite user.InviteCmd `cmd:"" help:"Invite users to your organization."`
	}
	ApiCmd struct {
		api.ApiCmd `cmd:"" help:"Interact with the Buildkite API"`
	}
)

func runCobraSystem() int {
	f, err := factory.New()
	if err != nil {
		handleError(bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite"))
		return bkErrors.ExitCodeInternalError
	}

	rootCmd, err := root.NewCmdRoot(f)
	if err != nil {
		handleError(bkErrors.NewInternalError(err, "failed to create commands", "This is likely a bug", "Report to Buildkite"))
		return bkErrors.ExitCodeInternalError
	}

	rootCmd.SetContext(context.Background())
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		handleError(err)
		return 1
	}
	return 0
}

func handleError(err error) {
	bkErrors.NewHandler().Handle(err)
}

func newKongParser(cli *CLI) (*kong.Kong, error) {
	return kong.New(
		cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{
			"output_default_format": string(output.DefaultFormat),
		},
	)
}

func main() {
	os.Exit(run())
}

func run() int {
	// Handle no-args case by showing help instead of error
	// This addresses the Kong limitation described in https://github.com/alecthomas/kong/issues/33
	if len(os.Args) <= 1 {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		_, _ = parser.Parse([]string{"--help"})
		return 0
	}

	// We can remove the isHelpRequest function when we have full Kong support for all commands
	if isHelpRequest() {
		return runCobraSystem()
	}

	cliInstance := &CLI{}

	parser, err := newKongParser(cliInstance)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	globals := cli.Globals{
		Yes:     cliInstance.Yes,
		NoInput: cliInstance.NoInput,
		Quiet:   cliInstance.Quiet,
	}

	ctx.BindTo(cli.GlobalFlags(globals), (*cli.GlobalFlags)(nil))

	if err := ctx.Run(cliInstance); err != nil {
		handleError(err)
		return 1
	}
	return 0
}

// We can rip this out when we have full Kong support for all commands
func isHelpRequest() bool {
	// Let Kong handle no-args case and global help
	if len(os.Args) < 2 {
		return false
	}

	// Global help, e.g. bk --help - let Kong handle this too
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		return false
	}

	// Let Kong handle build subcommand help since build is fully migrated
	if len(os.Args) >= 2 && os.Args[1] == "build" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "agent" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "job" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "cluster" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "artifacts" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "api" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "version" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "pipeline" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "whoami" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "init" {
		return false
	}

	if len(os.Args) >= 2 && os.Args[1] == "organization" {
		return false
	}
	if len(os.Args) >= 2 && os.Args[1] == "use" {
		return false
	}
	if len(os.Args) >= 2 && os.Args[1] == "package" {
		return false
	}
	if len(os.Args) >= 2 && os.Args[1] == "user" {
		return false
	}
	if len(os.Args) >= 2 && os.Args[1] == "configure" {
		return false
	}

	if len(os.Args) == 3 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
		return true
	}

	return false
}
