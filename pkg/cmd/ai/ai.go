package ai

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAI(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:     "ai <command>",
		Short:   "Manage AI integration.",
		Long:    "Work with Buildkite AI.",
		PreRunE: validation.OpenAITokenConfigured(f.Config),
		Example: heredoc.Doc(`
			# To configure your AI token
			$ bk ai configure
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(NewCmdAIConfigure(f))

	return &cmd
}
