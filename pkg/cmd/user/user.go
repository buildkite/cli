package user

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func CommandUser(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "user <command>",
		Short: "Manage users.",
		Long: heredoc.Doc(`
      Manage organization users via the CLI.

      To invite a user:
      bk user invite [email address]
      `),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(CommandUserInvite(f))

	return &cmd
}
