package main

import (
	"fmt"
	"os"

	"github.com/buildkite/buildkite-cli/buildkite"
	"github.com/buildkite/buildkite-cli/cli/commands"

	"github.com/99designs/keyring"
	"gopkg.in/alecthomas/kingpin.v2"
)

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

	configureCmd := app.Command("configure", "Configure aspects of buildkite cli")

	configureCmd.
		Command("default", "Default configuration flow").
		Default().
		Hidden().
		Action(func(c *kingpin.ParseContext) error {
			return commands.ConfigureDefaultCommand(commands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	configureBuildkiteCmd := configureCmd.
		Command("buildkite", "Configure buildkite.com authentication")

	configureBuildkiteCmd.
		Command("rest", "Configure buildkite.com authentication").
		Action(func(c *kingpin.ParseContext) error {
			return commands.ConfigureBuildkiteRestCommand(commands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	configureBuildkiteCmd.
		Command("graphql", "Configure buildkite.com graphql authentication (beta)").
		Action(func(c *kingpin.ParseContext) error {
			return commands.ConfigureBuildkiteGraphqlCommand(commands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	configureCmd.
		Command("github", "Configure github authentication").
		Action(func(c *kingpin.ParseContext) error {
			return commands.ConfigureGithubCommand(commands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	kingpin.MustParse(app.Parse(args))
}
