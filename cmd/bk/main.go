package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	kongCLI "github.com/buildkite/cli/v3/pkg/cmd/kong"
	"github.com/buildkite/cli/v3/pkg/cmd/root"
)

func main() {
	// Check if we should use Kong instead of Cobra
	if len(os.Args) > 1 && os.Args[1] == "--use-kong" {
		// Remove the --use-kong flag and run with Kong
		os.Args = append(os.Args[:1], os.Args[2:]...)
		code := mainRunKong()
		os.Exit(code)
	}

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

func mainRunKong() int {
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

	// Create Kong CLI
	cli := &kongCLI.CLI{}

	// Parse arguments
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": f.Version},
	)

	ctx2, err := parser.Parse(os.Args[1:])
	if err != nil {
		// If no command was provided, show help and return success like Cobra does
		if len(os.Args[1:]) == 0 {
			// Manually parse --help to show Kong's native help
			parser.Parse([]string{"--help"})
			return bkErrors.ExitCodeSuccess
		}

		// Handle other parsing errors
		errHandler := bkErrors.NewHandler().WithVerbose(cli.Verbose)
		errHandler.Handle(bkErrors.NewInternalError(
			err,
			"failed to parse command line arguments",
			"Check your command syntax",
			"Use --help for usage information",
		))
		return bkErrors.ExitCodeInternalError
	}

	// Execute the command
	err = ctx2.Run(ctx, f)
	if err != nil {
		// Handle execution errors
		errHandler := bkErrors.NewHandler().WithVerbose(cli.Verbose)
		errHandler.HandleWithDetails(err, "bk")
		return bkErrors.GetExitCodeForError(err)
	}

	return bkErrors.ExitCodeSuccess
}
