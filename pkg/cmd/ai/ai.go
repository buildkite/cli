package ai

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAI(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "ai <command>",
		Short: "Manage AI integration.",
		Long:  "Work with Buildkite AI.",
		Example: heredoc.Doc(`
			# To configure your AI token
			$ bk ai configure
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}

	cmd.AddCommand(NewCmdAIConfigure(f))

	return &cmd
}
