package main

import (
	"os"

	"github.com/buildkite/buildkite-cli/clicommands"
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

	clicommands.ConfigureGlobals(app)
	clicommands.ConfigureConfigureCommand(app)

	kingpin.MustParse(app.Parse(args))
}
