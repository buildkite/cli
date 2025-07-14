package main

import (
	"os"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func main() {
	code := mainRun()
	os.Exit(code)
}

func mainRun() int {
	// Create factory
	f, err := factory.New(version.Version)
	if err != nil {
		errHandler := bkErrors.NewHandler()
		errHandler.Handle(bkErrors.NewInternalError(
			err,
			"failed to initialize CLI",
			"This is likely a bug in the CLI",
			"Please report this issue to the Buildkite CLI team",
		))
		return bkErrors.ExitCodeInternalError
	}

	// Use Kong CLI
	return cli.Run(version.Version, f)
}
