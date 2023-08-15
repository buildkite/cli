package root

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/version"
	"github.com/spf13/cobra"
)

func NewCmdRoot() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use: "bk <command> <subcommand> [flags]",
		Short: "Buildkite CLI",
		Long: "Work with Buildkite from the command line.",
		Example: heredoc.Doc(`
			$ bk build view
		`),
	}

	cmd.PersistentFlags().Bool("help", false, "Show help for a command")

	cmd.AddCommand(version.NewCmdVersion())

	return cmd, nil
}
