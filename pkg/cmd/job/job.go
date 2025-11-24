package job

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdJob(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:               "job <command>",
		Short:             "Manage jobs within a build",
		Long:              "Manage jobs within a build",
		Example:           "$ bk job unblock 0190046e-e199-453b-a302-a21a4d649d31",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			f.SetGlobalFlags(cmd)
			return validation.CheckValidConfiguration(f.Config)(cmd, args)
		},
	}

	cmd.AddCommand(NewCmdJobList(f))
	cmd.AddCommand(NewCmdJobUnblock(f))
	cmd.AddCommand(NewCmdJobRetry(f))
	cmd.AddCommand(NewCmdJobCancel(f))

	return &cmd
}
