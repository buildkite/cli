package agent

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAgent(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "agent <subcommand> [flags]",
		Short: "",
		Long:  "",
	}

	cmd.AddCommand(NewCmdAgentStop(f))

	return &cmd
}
