package user

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func CommandUser(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "user <command>",
		Short: "Invite users to the organization",
		Long: heredoc.Doc(`
      Manage organization users via the CLI.

      To invite a user:
      bk user invite [email address]
      `),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}

	cmd.AddCommand(CommandUserInvite(f))

	return &cmd
}
