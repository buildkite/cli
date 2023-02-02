package main

import (
	"fmt"
	"os"

	"github.com/buildkite/cli/v2"
	"github.com/buildkite/cli/v2/graphql"
	"github.com/fatih/color"
	"golang.org/x/crypto/ssh/terminal"

	"gopkg.in/alecthomas/kingpin.v2"
)

// This is the main entry point for the bk cli tool. It handles all of the CLI wiring
// including defining commands arguments, flags, etc. These are all pushed into a
// struct per command and then delegated to a function in the commands package

func main() {
	run(os.Args[1:], os.Exit)
}

func run(args []string, exit func(int)) {
	app := kingpin.New(
		`bk`,
		`Manage buildkite from the command-line`,
	)

	app.Writer(os.Stdout)
	app.Version(cli.VersionString())
	app.Terminate(exit)

	// --------------------------
	//  global flags

	var (
		debug        bool
		debugGraphQL bool
	)

	app.Flag("debug", "Show debugging output").
		BoolVar(&debug)

	app.Flag("debug-graphql", "Show requests and responses for graphql").
		BoolVar(&debugGraphQL)

	app.PreAction(func(c *kingpin.ParseContext) (err error) {
		if debug {
			cli.Debug = true
		}
		if debugGraphQL {
			graphql.DebugHTTP = true
		}
		return err
	})

	// --------------------------
	// configure command

	configureCtx := cli.ConfigureCommandContext{}
	configureCmd := app.Command("configure", "Configure aspects of buildkite cli")

	configureCmd.
		Command("default", "Default configuration flow").
		Default().
		Hidden().
		Action(func(c *kingpin.ParseContext) error {
			configureCtx.Debug = debug
			configureCtx.TerminalContext = &cli.Terminal{}
			return cli.ConfigureDefaultCommand(configureCtx)
		})

	configureCmd.
		Command("buildkite", "Configure buildkite.com graphql authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureCtx.Debug = debug
			configureCtx.TerminalContext = &cli.Terminal{}
			return cli.ConfigureBuildkiteGraphQLCommand(configureCtx)
		})

	configureCmd.
		Command("github", "Configure github authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureCtx.Debug = debug
			configureCtx.TerminalContext = &cli.Terminal{}
			return cli.ConfigureGithubCommand(configureCtx)
		})

	// --------------------------
	// configure command

	initCtx := cli.InitCommandContext{}

	initCmd := app.
		Command("init", "Initialize a project in your filesystem for use with Buildkite").
		Action(func(c *kingpin.ParseContext) error {
			initCtx.Debug = debug
			initCtx.TerminalContext = &cli.Terminal{}
			return cli.InitCommand(initCtx)
		})

	initCmd.
		Flag("dir", "Directory of your project").
		ExistingDirVar(&initCtx.Dir)

	initCmd.
		Flag("pipeline", "Use a specific pipeline slug (org/pipeline)").
		StringVar(&initCtx.PipelineSlug)

	// --------------------------
	// build commands

	buildCmd := app.Command("build", "Operate on builds")

	buildCreateCtx := cli.BuildCreateCommandContext{
		Metadata: make(map[string]string),
	}
	buildCreateCmd := buildCmd.
		Command("create", "Create a new build in a pipeline").
		Action(func(c *kingpin.ParseContext) error {
			buildCreateCtx.Debug = debug
			buildCreateCtx.TerminalContext = &cli.Terminal{}

			// Default to the current directory
			if buildCreateCtx.PipelineSlug == "" && buildCreateCtx.Dir == "" {
				buildCreateCtx.Dir = "."
			}

			return cli.BuildCreateCommand(buildCreateCtx)
		})

	buildCreateCmd.
		Flag("dir", "Build a specific directory, defaults to the current").
		ExistingDirVar(&buildCreateCtx.Dir)

	buildCreateCmd.
		Flag("pipeline", "Build a specific pipeline rather than a directory").
		StringVar(&buildCreateCtx.PipelineSlug)

	buildCreateCmd.
		Flag("message", "The message to use for the build").
		StringVar(&buildCreateCtx.Message)

	buildCreateCmd.
		Flag("commit", "The commit to use for the build").
		StringVar(&buildCreateCtx.Commit)

	buildCreateCmd.
		Flag("branch", "The branch to use for the build").
		StringVar(&buildCreateCtx.Branch)

	buildCreateCmd.
		Flag("env", "Environment to pass to the build").
		StringsVar(&buildCreateCtx.Env)

	buildCreateCmd.
		Flag("meta-data", "Meta-data to pass to the build").
		StringMapVar(&buildCreateCtx.Metadata)

	// --------------------------
	// browse command

	browseCtx := cli.BrowseCommandContext{}

	browseCmd := app.
		Command("browse", "Open a pipeline on buildkite.com in your browser").
		Action(func(c *kingpin.ParseContext) error {
			browseCtx.Debug = debug
			browseCtx.TerminalContext = &cli.Terminal{}
			return cli.BrowseCommand(browseCtx)
		})

	browseCmd.
		Flag("dir", "Directory of your project").
		ExistingDirVar(&browseCtx.Dir)

	browseCmd.
		Flag("branch", "The branch to browse to").
		StringVar(&browseCtx.Branch)

	// --------------------------
	// pipeline commands

	pipelineCmd := app.Command("pipeline", "Operate on pipeline")

	pipelineListCtx := cli.PipelineListCommandContext{}
	pipelineListCmd := pipelineCmd.
		Command("list", "List buildkite pipelines").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			pipelineListCtx.Debug = debug
			pipelineListCtx.TerminalContext = &cli.Terminal{}
			return cli.PipelineListCommand(pipelineListCtx)
		})

	pipelineListCmd.
		Flag("fuzzy", "Fuzzy filter pipelines based on org and slug").
		StringVar(&pipelineListCtx.Fuzzy)

	pipelineListCmd.
		Flag("url", "Show buildkite.com urls for pipelines").
		BoolVar(&pipelineListCtx.ShowURL)

	pipelineListCmd.
		Flag("limit", "How many pipelines to output").
		IntVar(&pipelineListCtx.Limit)

	// --------------------------
	// artifact commands

	artifactCmd := app.Command("artifact", "Operate on artifacts")

	artifactDownloadCtx := cli.ArtifactDownloadCommandContext{}
	artifactDownloadCmd := artifactCmd.
		Command("download", "Download buildkite artifacts").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			artifactDownloadCtx.Debug = debug
			artifactDownloadCtx.TerminalContext = &cli.Terminal{}
			return cli.ArtifactDownloadCommand(artifactDownloadCtx)
		})

	artifactDownloadCmd.
		Flag("build", "Build to search for artifacts").
		StringVar(&artifactDownloadCtx.Build)

	artifactDownloadCmd.
		Flag("job", "Job to search for artifacts").
		StringVar(&artifactDownloadCtx.Job)

	artifactDownloadCmd.
		Arg("pattern", "Download only artifacts matching the glob pattern").
		StringVar(&artifactDownloadCtx.Pattern)

	// --------------------------
	// local command

	localCmd := app.Command("local", "Operate on your local repositories")

	var setupRunCmd = func(cmd *kingpin.CmdClause, runCmdCtx *cli.LocalRunCommandContext) {
		cmd.
			Flag("command", "The initial command to execute").
			Default("buildkite-agent pipeline upload").
			StringVar(&runCmdCtx.Command)

		cmd.
			Flag("filter", "A regex to filter step labels with").
			RegexpVar(&runCmdCtx.StepFilterRegex)

		cmd.
			Flag("dry-run", "Show what steps will be executed").
			BoolVar(&runCmdCtx.DryRun)

		cmd.
			Flag("prompt", "Prompt for each step before executing").
			BoolVar(&runCmdCtx.Prompt)

		cmd.
			Flag("env", "Environment to pass to the agent").
			Short('E').
			StringsVar(&runCmdCtx.Env)

		cmd.
			Flag("meta-data", "Meta-data to pass to the build").
			Short('M').
			StringMapVar(&runCmdCtx.Metadata)

		cmd.
			Flag("listen-port", "A specific port for the local API server to listen on").
			IntVar(&runCmdCtx.ListenPort)

		cmd.
			Arg("file", "A specific pipeline file to upload").
			FileVar(&runCmdCtx.File)
	}

	localRunCmdCtx := cli.LocalRunCommandContext{
		Metadata: make(map[string]string),
	}
	localRunCmd := localCmd.
		Command("run", "Run a pipeline locally").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			localRunCmdCtx.Debug = debug
			localRunCmdCtx.TerminalContext = &cli.Terminal{}
			return cli.LocalRunCommand(localRunCmdCtx)
		})

	setupRunCmd(localRunCmd, &localRunCmdCtx)

	runCmdCtx := cli.LocalRunCommandContext{}
	runCmd := app.
		Command("run", "Run a pipeline locally (alias for local run)").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			runCmdCtx.Debug = debug
			runCmdCtx.TerminalContext = &cli.Terminal{}
			return cli.LocalRunCommand(runCmdCtx)
		})

	setupRunCmd(runCmd, &runCmdCtx)

	// --------------------------
	// run the app, parse args

	if _, err := app.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, color.RedString("🚨 %v\n", err))

		if ec, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(ec.ExitCode())
		} else {
			os.Exit(1)
		}
	}
}

func terminalPrompt(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(b), nil
}
