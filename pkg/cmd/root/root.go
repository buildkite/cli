package root

import (
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/config"
	authCmd "github.com/buildkite/cli/v3/pkg/cmd/auth"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdRoot(viper *viper.Viper, version string) (*cobra.Command, error) {
	cobra.OnInitialize(initConfig)

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
	cmd.AddCommand(authCmd.NewCmdAuth(viper))

	return cmd, nil
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetConfigFile(config.ConfigFile())

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
