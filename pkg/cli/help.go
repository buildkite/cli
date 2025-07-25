package cli

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/pkg/factory"
)

// Help command provides an alternative to flag-based help for better discoverability
// Examples:
//
//	bk help             - Show general help
//	bk help build       - Show help for build command
//	bk help build new   - Show help for build new subcommand
type HelpCmd struct {
	Commands []string `arg:"" optional:"" help:"Commands to show help for"`
}

func (h *HelpCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Build help args - if no commands specified, show main help
	helpArgs := []string{"--help"}

	// If commands are provided, add them before --help
	if len(h.Commands) > 0 {
		helpArgs = append(h.Commands, "--help")
	}

	// Re-parse with help args to trigger help display
	var cli CLI
	parser, err := kong.New(
		&cli,
		kong.Name("bk"),
		kong.Description("The official Buildkite CLI"),
		kong.Vars{"version": f.Version},
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.BindTo(f, (**factory.Factory)(nil)),
		kong.UsageOnError(),
	)
	if err != nil {
		return err
	}

	// Parse the help args to trigger help display
	kongCtx, err := parser.Parse(helpArgs)
	if err != nil {
		// Check if this is an "unexpected argument" error - show help instead
		if errMsg := err.Error(); len(errMsg) > 20 && errMsg[:20] == "unexpected argument " {
			// Show the error message first
			fmt.Fprintf(parser.Stderr, "Error: %s\n\n", errMsg)

			// For subcommand errors, try to show subcommand help first
			if len(h.Commands) > 1 {
				helpArgs := []string{h.Commands[0], "--help"}
				if helpCtx, helpErr := parser.Parse(helpArgs); helpErr == nil {
					_ = helpCtx.Run()
					return nil
				}
			}
			// Fall back to main help
			if helpCtx, helpErr := parser.Parse([]string{"--help"}); helpErr == nil {
				_ = helpCtx.Run()
			}
			return nil
		}
		// Kong will show help and return an error - we can ignore this specific error
		return nil
	}

	// If parsing succeeds, run the context (which will show help)
	err = kongCtx.Run()
	if err != nil {
		return nil
	}

	return nil
}
