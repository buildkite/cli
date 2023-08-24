package root

import (
	"github.com/MakeNowJust/heredoc"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdRoot(viper *viper.Viper, version string) (*cobra.Command, error) {
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

	cmd.AddCommand(versionCmd.NewCmdVersion())
	cmd.AddCommand(configureCmd.NewCmdConfigure(viper))

	return cmd, nil
}
