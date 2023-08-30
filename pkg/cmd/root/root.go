package root

import (
	"github.com/MakeNowJust/heredoc"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
	"github.com/spf13/cobra"
)

func NewCmdRoot(f *factory.Factory) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "bk <command> <subcommand> [flags]",
		Short: "Buildkite CLI",
		Long:  "Work with Buildkite from the command line.",
		Example: heredoc.Doc(`
			$ bk build view
		`),
		Annotations: map[string]string{
			"versionInfo": versionCmd.Format(f.Version),
		},
	}

	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(versionCmd.NewCmdVersion())

	return cmd, nil
}
