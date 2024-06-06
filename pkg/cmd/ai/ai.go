package ai

import (
	"github.com/MakeNowJust/heredoc"
	aiUse "github.com/buildkite/cli/v3/pkg/cmd/ai/use"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
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
	}

	cmd.AddCommand(NewCmdAIConfigure(f))
	cmd.AddCommand(aiUse.NewCmdUse(f))

	return &cmd
}
