package auth

import (
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/auth/init"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCmdAuth(viper *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Manage authentication for bk",
	}

	cmd.AddCommand(initCmd.NewCmdInit(viper))

	return cmd
}
