package main

import (
	"fmt"
	"os"

	"github.com/buildkite/cli"
	"github.com/buildkite/cli/graphql"
	"github.com/fatih/color"

	"github.com/99designs/keyring"
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

	backendsAvailable := []string{}
	for _, backendType := range keyring.AvailableBackends() {
		backendsAvailable = append(backendsAvailable, string(backendType))
	}

	var (
		debug          bool
		debugGraphQL   bool
		keyringBackend string
		keyringImpl    keyring.Keyring
	)

	app.Flag("debug", "Show debugging output").
		BoolVar(&debug)

	app.Flag("debug-graphql", "Show requests and responses for graphql").
		BoolVar(&debugGraphQL)

	app.Flag("keyring-backend", fmt.Sprintf("Keyring backend to use: %v", backendsAvailable)).
		OverrideDefaultFromEnvar("BUILDKITE_CLI_KEYRING_BACKEND").
		EnumVar(&keyringBackend, backendsAvailable...)

	app.PreAction(func(c *kingpin.ParseContext) (err error) {
		if debug {
			keyring.Debug = true
			cli.Debug = true
		}
		if debugGraphQL {
			graphql.DebugHTTP = true
		}
		keyringImpl, err = keyring.Open(keyring.Config{
			ServiceName: "buildkite",
		})
		if err != nil {
			return err
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
			configureCtx.Keyring = keyringImpl
			configureCtx.TerminalContext = &cli.Terminal{}
			return cli.ConfigureDefaultCommand(configureCtx)
		})

	configureCmd.
		Command("buildkite", "Configure buildkite.com graphql authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureCtx.Debug = debug
			configureCtx.Keyring = keyringImpl
			configureCtx.TerminalContext = &cli.Terminal{}
			return cli.ConfigureBuildkiteGraphQLCommand(configureCtx)
		})

	configureCmd.
		Command("github", "Configure github authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureCtx.Debug = debug
			configureCtx.Keyring = keyringImpl
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
			initCtx.Keyring = keyringImpl
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

	buildCreateCtx := cli.BuildCreateCommandContext{}
	buildCreateCmd := buildCmd.
		Command("create", "Create a new build in a pipeline").
		Action(func(c *kingpin.ParseContext) error {
			buildCreateCtx.Debug = debug
			buildCreateCtx.Keyring = keyringImpl
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

	// --------------------------
	// browse command

	browseCtx := cli.BrowseCommandContext{}

	browseCmd := app.
		Command("browse", "Open a pipeline on buildkite.com in your browser").
		Action(func(c *kingpin.ParseContext) error {
			browseCtx.Debug = debug
			browseCtx.Keyring = keyringImpl
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
	// list command

	listCmd := app.Command("list", "List various things")

	listPipelinesCtx := cli.ListPipelinesCommandContext{}
	listPipelinesCmd := listCmd.
		Command("pipelines", "List buildkite pipelines").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			listPipelinesCtx.Debug = debug
			listPipelinesCtx.Keyring = keyringImpl
			listPipelinesCtx.TerminalContext = &cli.Terminal{}
			return cli.ListPipelinesCommand(listPipelinesCtx)
		})

	listPipelinesCmd.
		Flag("fuzzy", "Fuzzy filter pipelines based on org and slug").
		StringVar(&listPipelinesCtx.Fuzzy)

	listPipelinesCmd.
		Flag("url", "Show buildkite.com urls for pipelines").
		BoolVar(&listPipelinesCtx.ShowURL)

	listPipelinesCmd.
		Flag("limit", "How many pipelines to output").
		IntVar(&listPipelinesCtx.Limit)

	// --------------------------
	// run command

	runCmd := app.Command("run", "Run builds")

	runLocalCmdCtx := cli.RunLocalCommandContext{}
	runLocalCmd := runCmd.
		Command("local", "Run a pipeline locally").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			runLocalCmdCtx.Debug = debug
			runLocalCmdCtx.Keyring = keyringImpl
			runLocalCmdCtx.TerminalContext = &cli.Terminal{}
			return cli.RunLocalCommand(runLocalCmdCtx)
		})

	runLocalCmd.
		Flag("command", "The initial command to execute").
		Default("buildkite-agent pipeline upload").
		StringVar(&runLocalCmdCtx.Command)

	runLocalCmd.
		Flag("filter", "A regex to filter step labels with").
		RegexpVar(&runLocalCmdCtx.StepFilterRegex)

	runLocalCmd.
		Flag("dry-run", "Show what steps will be executed").
		BoolVar(&runLocalCmdCtx.DryRun)

	runLocalCmd.
		Flag("prompt", "Prompt for each step before executing").
		BoolVar(&runLocalCmdCtx.Prompt)

	runLocalCmd.
		Flag("env", "Environment to pass to the agent").
		Short('E').
		StringsVar(&runLocalCmdCtx.Env)

	runLocalCmd.
		Arg("file", "A specific pipeline file to upload").
		FileVar(&runLocalCmdCtx.File)


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
