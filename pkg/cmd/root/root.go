package root

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	agentCmd "github.com/buildkite/cli/v3/pkg/cmd/agent"
	artifactsCmd "github.com/buildkite/cli/v3/pkg/cmd/artifacts"
	buildCmd "github.com/buildkite/cli/v3/pkg/cmd/build"
	clusterCmd "github.com/buildkite/cli/v3/pkg/cmd/cluster"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pipelineCmd "github.com/buildkite/cli/v3/pkg/cmd/pipeline"
	"github.com/spf13/cobra"
)

func NewCmdRoot(f *factory.Factory) (*cobra.Command, error) {
	var verbose bool

	cmd := &cobra.Command{
		Use:          "bk <command> <subcommand> [flags]",
		Short:        "Buildkite CLI",
		Long:         "Work with Buildkite from the command line.",
		SilenceUsage: true,
		Example: heredoc.Doc(`
			$ bk build view
		`),
		Annotations: map[string]string{
			"versionInfo": fmt.Sprintf("bk version %s", f.Version),
		},
		Run: func(cmd *cobra.Command, args []string) {
			versionFlag, _ := cmd.Flags().GetBool("version")
			if versionFlag {
				fmt.Printf("bk version %s\n", f.Version)
				return
			}
			// If --version flag is not used, show help
			_ = cmd.Help()
		},
		// This function will run after command execution and handle any errors
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			// This will be overridden by ExecuteWithErrorHandling
			return nil
		},
	}

	cmd.AddCommand(agentCmd.NewCmdAgent(f))
	cmd.AddCommand(artifactsCmd.NewCmdArtifacts(f))
	cmd.AddCommand(buildCmd.NewCmdBuild(f))
	cmd.AddCommand(clusterCmd.NewCmdCluster(f))
	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(pipelineCmd.NewCmdPipeline(f))

	cmd.Flags().BoolP("version", "v", false, "Print the version number")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Enable verbose error output")

	return cmd, nil
}
