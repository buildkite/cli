package main

import (
	"context"
	"os"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
)

func main() {
	code := mainRun()
	os.Exit(code)
}

func mainRun() int {
	ctx := context.Background()

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

	// Create root command
	rootCmd, err := root.NewCmdRoot(f)
	if err != nil {
		errHandler := bkErrors.NewHandler()
		errHandler.Handle(bkErrors.NewInternalError(
			err,
			"failed to create command structure",
			"This is likely a bug in the CLI",
			"Please report this issue to the Buildkite CLI team",
		))
		return bkErrors.ExitCodeInternalError
	}

	// Set context
	rootCmd.SetContext(ctx)

	// Get verbose flag value
	verbose, _ := rootCmd.PersistentFlags().GetBool("verbose")

	// Silence Cobra's error printing so we can handle it ourselves
	rootCmd.SilenceErrors = true

	// Execute the command
	err = rootCmd.Execute()
	if err != nil {
		// Handle the error with our error handler
		errHandler := bkErrors.NewHandler().WithVerbose(verbose)
		errHandler.HandleWithDetails(err, rootCmd.CommandPath())
		return bkErrors.GetExitCodeForError(err)
	}

	// If we get here, the command executed successfully
	return bkErrors.ExitCodeSuccess
}
