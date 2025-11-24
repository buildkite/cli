package pkg

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdPackage(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:     "package <command>",
		Aliases: []string{"pkg"},
		Short:   "Manage packages",
		Long:    "Work with Buildkite Package Registries",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			f.SetGlobalFlags(cmd)
			return validation.CheckValidConfiguration(f.Config)(cmd, args)
		},
	}

	cmd.AddCommand(NewCmdPackagePush(f))
	return &cmd
}
