package clicommands

import (
	"fmt"

	"github.com/99designs/keyring"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	keyringImpl keyring.Keyring
)

var GlobalFlags struct {
	Debug          bool
	KeyringBackend string
}

func ConfigureGlobals(app *kingpin.Application) {
	backendsAvailable := []string{}
	for _, backendType := range keyring.AvailableBackends() {
		backendsAvailable = append(backendsAvailable, string(backendType))
	}

	app.Flag("debug", "Show debugging output").
		BoolVar(&GlobalFlags.Debug)

	app.Flag("keyring-backend", fmt.Sprintf("Keyring backend to use: %v", backendsAvailable)).
		OverrideDefaultFromEnvar("BUILDKITE_CLI_KEYRING_BACKEND").
		EnumVar(&GlobalFlags.KeyringBackend, backendsAvailable...)

	app.PreAction(func(c *kingpin.ParseContext) (err error) {
		if GlobalFlags.Debug {
			keyring.Debug = true
		}
		if keyringImpl == nil {
			keyringImpl, err = keyring.Open(keyring.Config{
				ServiceName: "buildkite",
			})
		}
		return err
	})

}
