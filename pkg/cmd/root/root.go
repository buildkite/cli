package root

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	packageCmd "github.com/buildkite/cli/v3/pkg/cmd/pkg"
	promptCmd "github.com/buildkite/cli/v3/pkg/cmd/prompt"
	useCmd "github.com/buildkite/cli/v3/pkg/cmd/use"
	"github.com/buildkite/cli/v3/pkg/cmd/user"
	"github.com/spf13/cobra"
)

func NewCmdRoot(f *factory.Factory) (*cobra.Command, error) {
	var skipConfirm bool
	var noInput bool
	var quiet bool

	cmd := &cobra.Command{
		Use:              "bk <command> <subcommand> [flags]",
		Short:            "Buildkite CLI",
		Long:             "Work with Buildkite from the command line.",
		SilenceUsage:     true,
		TraverseChildren: true,
		Example: heredoc.Doc(`
			$ bk build view
		`),
		Run: func(cmd *cobra.Command, args []string) {
			versionFlag, _ := cmd.Flags().GetBool("version")
			if versionFlag {
				fmt.Printf("bk version %s\n\n", f.Version)
				return
			}
			// If --version flag is not used, show help
			_ = cmd.Help()
		},
		// This function will run after command execution and handle any errors
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			// This will be overridden by ExecuteWithErrorHandling
			return nil
		},
	}

	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(packageCmd.NewCmdPackage(f))
	cmd.AddCommand(promptCmd.NewCmdPrompt(f))
	cmd.AddCommand(useCmd.NewCmdUse(f))
	cmd.AddCommand(user.CommandUser(f))

	cmd.Flags().BoolP("version", "v", false, "Print the version number")
	// Global flags for automation and scripting
	// NOTE: Due to Cobra, these must come AFTER a subcommand (e.g., 'bk job --yes cancel')
	// Once migrated to Kong, they'll work anywhere (e.g., 'bk --yes job cancel')
	cmd.PersistentFlags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip all confirmation prompts (useful for automation)")
	cmd.PersistentFlags().BoolVar(&noInput, "no-input", false, "Disable all interactive prompts (fail if input is required)")
	cmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all progress output")

	// Set factory flags before any command runs
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		f.SkipConfirm = skipConfirm
		f.NoInput = noInput
		f.Quiet = quiet
		return nil
	}

	return cmd, nil
}
