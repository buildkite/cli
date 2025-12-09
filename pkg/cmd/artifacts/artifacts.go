package artifacts

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdArtifacts(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "artifacts <command>",
		Args:  cobra.ArbitraryArgs,
		Long:  "Manage pipeline build artifacts",
		Short: "Manage pipeline build artifacts",
		Example: heredoc.Doc(`
			# To view pipeline build artifacts
			$ bk artifacts list [build number] [flags]

			# To download a specific artifact
			$ bk artifacts download <artifact UUID>
		`),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			f.SetGlobalFlags(cmd)
			return validation.CheckValidConfiguration(f.Config)(cmd, args)
		},
	}

	return &cmd
}
