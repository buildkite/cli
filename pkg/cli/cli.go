package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/posener/complete"
	"github.com/willabides/kongplete"
)

// GlobalOptions contains global flags available across all commands
type GlobalOptions struct {
	// Output flag removed - now only available on data query commands
}

// CLI represents the entire command structure
type CLI struct {
	GlobalOptions

	Verbose bool `short:"V" help:"Enable verbose error output"`

	Agent              AgentCmd                     `cmd:"" help:"Manage agents (list, stop, view agent details)"`
	API                APICmd                       `cmd:"" help:"Make raw API requests to Buildkite REST API"`
	Artifacts          ArtifactsCmd                 `cmd:"" help:"Work with build artifacts (list, download)"`
	Build              BuildCmd                     `cmd:"" help:"Manage builds (create, view, cancel, rebuild, watch status)"`
	Cluster            ClusterCmd                   `cmd:"" help:"Manage clusters (list, view cluster details)"`
	Configure          ConfigureCmd                 `cmd:"" help:"Configure authentication credentials and API tokens (run 'bk configure main --help' for all options)"`
	Init               InitCmd                      `cmd:"" help:"Create .buildkite/pipeline.yml with a basic build step interactively"`
	Job                JobCmd                       `cmd:"" help:"Manage individual build jobs (retry, unblock)"`
	Pipeline           PipelineCmd                  `cmd:"" help:"Manage pipelines (create, view details, validate configuration)"`
	Package            PackageCmd                   `name:"package" cmd:"" help:"Manage packages in Buildkite package registries" aliases:"pkg"`
	Prompt             PromptCmd                    `cmd:"" help:"Generate shell prompt integration to display current Buildkite organization."`
	Use                UseCmd                       `cmd:"" help:"Switch between different Buildkite organizations"`
	User               UserCmd                      `cmd:"" help:"Manage users (invite users to organization via email)"`
	VersionCmd         VersionSub                   `name:"version" cmd:"" help:"Show CLI version information"`
	Help               HelpCmd                      `cmd:"" help:"Show detailed help for commands and subcommands"`
	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Install shell tab completion for bash, zsh, fish, or powershell"`
}

// Run executes the CLI with Kong
func Run(version string, f *factory.Factory) int {
	// Grab original args once
	args := os.Args[1:]

	// Handle the simple "bk -v/--version" shortcut early and exit
	if len(args) == 1 {
		switch args[0] {
		case "--version", "-v":
			fmt.Printf("bk version %s\n", version)
			return 0
		}
	}

	ctx := context.Background()
	var cli CLI
	handler := bkErrors.NewHandler()

	parser, err := kong.New(
		&cli,
		kong.Name("bk"),
		kong.Description("The official Buildkite CLI"),
		kong.Vars{"version": version},
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.BindTo(f, (**factory.Factory)(nil)),
		kong.UsageOnError(), // show help when something is wrong

	)
	if err != nil {
		handler.Handle(err)
		return bkErrors.ExitCodeInternalError
	}

	// Handle shell completion requests
	kongplete.Complete(parser,
		kongplete.WithPredictor("file", complete.PredictFiles("*")),
		kongplete.WithPredictor("dir", complete.PredictDirs("*")),
	)

	// No command? -> show help instead of "expected one of ..."
	if len(args) == 0 {
		args = []string{"--help"}
	}

	// Parse the arguments we decided on
	kongCtx, err := parser.Parse(args)
	if err != nil {
		// Check if this is an "unexpected argument" error - show help instead
		if errMsg := err.Error(); len(errMsg) > 20 && errMsg[:20] == "unexpected argument " {
			// Show the error message first
			fmt.Fprintf(parser.Stderr, "Error: %s\n\n", errMsg)

			// For subcommand errors, try to show subcommand help first
			if len(args) > 1 {
				helpArgs := []string{args[0], "--help"}
				if helpCtx, helpErr := parser.Parse(helpArgs); helpErr == nil {
					_ = helpCtx.Run()
					return 1
				}
			}
			// Fall back to main help
			if helpCtx, helpErr := parser.Parse([]string{"--help"}); helpErr == nil {
				_ = helpCtx.Run()
			}
			return 1
		}
		FormatError(err, "table", cli.Verbose)
		return bkErrors.GetExitCodeForError(err)
	}

	if err := kongCtx.Run(); err != nil {
		FormatError(err, "table", cli.Verbose)
		return bkErrors.GetExitCodeForError(err)
	}

	return 0
}
