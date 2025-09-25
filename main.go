package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
)

// Kong CLI structure, with base commands defined as additional commands are defined in their respective files
type CLI struct {
	Agent     AgentCmd     `cmd:"" help:"Manage agents"`
	Api       ApiCmd       `cmd:"" help:"Interact with the Buildkite API"`
	Artifacts ArtifactsCmd `cmd:"" help:"Manage pipeline build artifacts"`
	Build     BuildCmd     `cmd:"" help:"Manage pipeline builds"`
	Cluster   ClusterCmd   `cmd:"" help:"Manage organization clusters"`
	Configure ConfigureCmd `cmd:"" help:"Configure Buildkite API token"`
	Init      InitCmd      `cmd:"" help:"Initialize a pipeline.yaml file"`
	Job       JobCmd       `cmd:"" help:"Manage jobs within a build"`
	Pipeline  PipelineCmd  `cmd:"" help:"Manage pipelines"`
	Package   PackageCmd   `cmd:"" help:"Manage packages"`
	Use       UseCmd       `cmd:"" help:"Select an organization"`
	User      UserCmd      `cmd:"" help:"Invite users to the organization"`
	Version   VersionCmd   `cmd:"" help:"Print the version of the CLI being used"`
}

// Hybrid delegation commands, we should delete from these when native Kong implementations ready
type (
	VersionCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	AgentCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	ArtifactsCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	BuildCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	ClusterCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	JobCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	PackageCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	PipelineCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	UserCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	ApiCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	ConfigureCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	InitCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
	UseCmd struct {
		Args []string `arg:"" optional:"" passthrough:"all"`
	}
)

// Delegation methods, we should delete when native Kong implementations ready
func (v *VersionCmd) Run(*CLI) error   { return delegateToCobraSystem("version", v.Args) }
func (a *AgentCmd) Run(*CLI) error     { return delegateToCobraSystem("agent", a.Args) }
func (a *ArtifactsCmd) Run(*CLI) error { return delegateToCobraSystem("artifacts", a.Args) }
func (b *BuildCmd) Run(*CLI) error     { return delegateToCobraSystem("build", b.Args) }
func (c *ClusterCmd) Run(*CLI) error   { return delegateToCobraSystem("cluster", c.Args) }
func (j *JobCmd) Run(*CLI) error       { return delegateToCobraSystem("job", j.Args) }
func (p *PackageCmd) Run(*CLI) error   { return delegateToCobraSystem("package", p.Args) }
func (p *PipelineCmd) Run(*CLI) error  { return delegateToCobraSystem("pipeline", p.Args) }
func (u *UserCmd) Run(*CLI) error      { return delegateToCobraSystem("user", u.Args) }
func (a *ApiCmd) Run(*CLI) error       { return delegateToCobraSystem("api", a.Args) }
func (c *ConfigureCmd) Run(*CLI) error { return delegateToCobraSystem("configure", c.Args) }
func (i *InitCmd) Run(*CLI) error      { return delegateToCobraSystem("init", i.Args) }
func (u *UseCmd) Run(*CLI) error       { return delegateToCobraSystem("use", u.Args) }

func delegateToCobraSystem(command string, args []string) error {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = append([]string{os.Args[0], command}, args...)

	if code := runCobraSystem(); code != 0 {
		os.Exit(code)
	}
	return nil
}

func runCobraSystem() int {
	f, err := factory.New(version.Version)
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

func main() {
	os.Exit(run())
}

func run() int {
	// We can remove the isHelpRequest function when we have full Kong support for all commands
	if isHelpRequest() {
		return runCobraSystem()
	}

	cli := &CLI{}
	parser, err := kong.New(cli, kong.Description("Buildkite CLI"), kong.NoDefaultHelp())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err := ctx.Run(cli); err != nil {
		handleError(err)
		return 1
	}
	return 0
}

// We can rip this out when we have full Kong support for all commands
func isHelpRequest() bool {
	if len(os.Args) < 2 {
		return false
	}

	// Global help, e.g. bk --help
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		return true
	}

	// Subcommand help, e.g. bk agent --help
	if len(os.Args) == 3 && (os.Args[2] == "-h" || os.Args[2] == "--help") {
		return true
	}

	return false
}
