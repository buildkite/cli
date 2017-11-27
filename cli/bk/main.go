package main

import (
	"fmt"
	"os"

	"github.com/buildkite/buildkite-cli/clicommands"

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
	app.Version(Version)
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

	configureCmd := app.Command("configure", "Configure bk")

	configureCmd.
		Command("all", "Configure buildkite cli").
		Default().
		Action(func(c *kingpin.ParseContext) error {
			return clicommands.ConfigureCommand(clicommands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	configureCmd.
		Command("buildkite", "Configure buildkite.com authentication").
		Action(func(c *kingpin.ParseContext) error {
			return clicommands.ConfigureBuildkiteCommand(clicommands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	configureCmd.
		Command("github", "Configure github authentication").
		Action(func(c *kingpin.ParseContext) error {
			return clicommands.ConfigureGithubCommand(clicommands.ConfigureCommandInput{
				Keyring: keyringImpl,
				Debug:   debug,
			})
		})

	kingpin.MustParse(app.Parse(args))
}
