package job

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdJobUnblock(f *factory.Factory) *cobra.Command {
	return &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "unblock <job id>",
		Short:                 "Unblock a job",
		Long:                  "Unblock a job",
		Args:                  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
