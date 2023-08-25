package root

import (
	"github.com/MakeNowJust/heredoc"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdRoot(v *viper.Viper, version string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "bk <command> <subcommand> [flags]",
		Short: "Buildkite CLI",
		Long:  "Work with Buildkite from the command line.",
		Example: heredoc.Doc(`
			$ bk build view
		`),
		Annotations: map[string]string{
			"versionInfo": versionCmd.Format(version),
		},
	}

	cmd.AddCommand(configureCmd.NewCmdConfigure(v))
	cmd.AddCommand(initCmd.NewCmdInit(v))
	cmd.AddCommand(versionCmd.NewCmdVersion())

	return cmd, nil
}
