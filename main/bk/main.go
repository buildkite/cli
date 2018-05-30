package main

import (
	"fmt"
	"os"

	"github.com/buildkite/cli/cmd"
	"github.com/buildkite/cli/pkg"

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
	app.Version(buildkite.VersionString())
	app.Terminate(exit)

	// --------------------------
	//  global flags

	backendsAvailable := []string{}
	for _, backendType := range keyring.AvailableBackends() {
		backendsAvailable = append(backendsAvailable, string(backendType))
	}

	var (
		debug          bool
		keyringBackend string
		keyringImpl    keyring.Keyring
	)

	app.Flag("debug", "Show debugging output").
		BoolVar(&debug)

	app.Flag("keyring-backend", fmt.Sprintf("Keyring backend to use: %v", backendsAvailable)).
		OverrideDefaultFromEnvar("BUILDKITE_CLI_KEYRING_BACKEND").
		EnumVar(&keyringBackend, backendsAvailable...)

	app.PreAction(func(c *kingpin.ParseContext) (err error) {
		if debug {
			keyring.Debug = true
			commands.Debug = true
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

	configureInput := commands.ConfigureCommandInput{}

	configureCmd := app.Command("configure", "Configure aspects of buildkite cli")

	configureCmd.
		Command("default", "Default configuration flow").
		Default().
		Hidden().
		Action(func(c *kingpin.ParseContext) error {
			configureInput.Debug = debug
			configureInput.Keyring = keyringImpl
			return commands.ConfigureDefaultCommand(configureInput)
		})

	configureCmd.
		Command("buildkite", "Configure buildkite.com graphql authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureInput.Debug = debug
			configureInput.Keyring = keyringImpl
			return commands.ConfigureBuildkiteGraphqlCommand(configureInput)
		})

	configureCmd.
		Command("github", "Configure github authentication").
		Action(func(c *kingpin.ParseContext) error {
			configureInput.Debug = debug
			configureInput.Keyring = keyringImpl
			return commands.ConfigureGithubCommand(configureInput)
		})

	// --------------------------
	// configure command

	initInput := commands.InitCommandInput{}

	initCmd := app.
		Command("init", "Initialize a project in your filesystem for use with Buildkite").
		Action(func(c *kingpin.ParseContext) error {
			initInput.Debug = debug
			initInput.Keyring = keyringImpl
			return commands.InitCommand(initInput)
		})

	initCmd.
		Arg("dir", "Directory of your project").
		Default(".").
		ExistingDirVar(&initInput.Dir)

	kingpin.MustParse(app.Parse(args))
}
