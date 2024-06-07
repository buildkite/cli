package ai

import (
	"errors"

	addCmd "github.com/buildkite/cli/v3/pkg/cmd/ai/add"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAIConfigure(f *factory.Factory) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "configure",
		Aliases: []string{"config"},
		Args:    cobra.NoArgs,
		Short:   "Configure OpenAI token.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && f.Config.GetOpenAIToken() != "" {
				return errors.New("OpenAI token already configured. You must use --force.")
			}

			return addCmd.ConfigureRun(f)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")

	cmd.AddCommand(addCmd.NewCmdAIAdd(f))

	return cmd
}
