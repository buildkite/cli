package main

import (
	"fmt"
	"os"

	"github.com/buildkite/cli"
	"github.com/buildkite/cli/graphql"

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
		Arg("dir", "Directory of your project").
		Default(".").
		ExistingDirVar(&initCtx.Dir)

	// --------------------------
	// configure command

	buildCtx := cli.BuildCommandContext{}

	buildCmd := app.
		Command("build", "Trigger a Buildkite build").
		Action(func(c *kingpin.ParseContext) error {
			buildCtx.Debug = debug
			buildCtx.Keyring = keyringImpl
			buildCtx.TerminalContext = &cli.Terminal{}
			return cli.BuildCommand(buildCtx)
		})

	buildCmd.
		Arg("dir", "Directory of your project").
		Default(".").
		ExistingDirVar(&buildCtx.Dir)

	kingpin.MustParse(app.Parse(args))
}
