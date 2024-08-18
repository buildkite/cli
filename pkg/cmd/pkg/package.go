package pkg

import (
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdPackage(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:               "package <command>",
		Short:             "Manage packages",
		Long:              "Work with Buildkite pipelines.",
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}

	cmd.AddCommand(NewCmdPackagePush(f))
	return &cmd
}
