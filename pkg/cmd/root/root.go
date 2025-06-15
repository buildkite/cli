package root

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	agentCmd "github.com/buildkite/cli/v3/pkg/cmd/agent"
	apiCmd "github.com/buildkite/cli/v3/pkg/cmd/api"
	artifactsCmd "github.com/buildkite/cli/v3/pkg/cmd/artifacts"
	buildCmd "github.com/buildkite/cli/v3/pkg/cmd/build"
	clusterCmd "github.com/buildkite/cli/v3/pkg/cmd/cluster"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	docsCmd "github.com/buildkite/cli/v3/pkg/cmd/docs"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	jobCmd "github.com/buildkite/cli/v3/pkg/cmd/job"
	pipelineCmd "github.com/buildkite/cli/v3/pkg/cmd/pipeline"
	packageCmd "github.com/buildkite/cli/v3/pkg/cmd/pkg"
	promptCmd "github.com/buildkite/cli/v3/pkg/cmd/prompt"
	useCmd "github.com/buildkite/cli/v3/pkg/cmd/use"
	"github.com/buildkite/cli/v3/pkg/cmd/user"
	versionCmd "github.com/buildkite/cli/v3/pkg/cmd/version"
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
			"versionInfo": versionCmd.Format(f.Version),
		},
		Run: func(cmd *cobra.Command, args []string) {
			versionFlag, _ := cmd.Flags().GetBool("version")
			if versionFlag {
				fmt.Println(versionCmd.Format(f.Version))
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
	cmd.AddCommand(apiCmd.NewCmdAPI(f))
	cmd.AddCommand(artifactsCmd.NewCmdArtifacts(f))
	cmd.AddCommand(buildCmd.NewCmdBuild(f))
	cmd.AddCommand(clusterCmd.NewCmdCluster(f))
	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(docsCmd.NewCmdDocs(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(jobCmd.NewCmdJob(f))
	cmd.AddCommand(packageCmd.NewCmdPackage(f))
	cmd.AddCommand(pipelineCmd.NewCmdPipeline(f))
	cmd.AddCommand(promptCmd.NewCmdPrompt(f))
	cmd.AddCommand(useCmd.NewCmdUse(f))
	cmd.AddCommand(user.CommandUser(f))
	cmd.AddCommand(versionCmd.NewCmdVersion(f))

	cmd.Flags().BoolP("version", "v", false, "Print the version number")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Enable verbose error output")

	return cmd, nil
}
