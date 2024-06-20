package root

import (
	"github.com/MakeNowJust/heredoc"
	agentCmd "github.com/buildkite/cli/v3/pkg/cmd/agent"
	aiCmd "github.com/buildkite/cli/v3/pkg/cmd/ai"
	buildCmd "github.com/buildkite/cli/v3/pkg/cmd/build"
	clusterCmd "github.com/buildkite/cli/v3/pkg/cmd/cluster"
	configureCmd "github.com/buildkite/cli/v3/pkg/cmd/configure"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	initCmd "github.com/buildkite/cli/v3/pkg/cmd/init"
	jobCmd "github.com/buildkite/cli/v3/pkg/cmd/job"
	pipelineCmd "github.com/buildkite/cli/v3/pkg/cmd/pipeline"
	useCmd "github.com/buildkite/cli/v3/pkg/cmd/use"
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
		SilenceUsage: true,
	}

	cmd.AddCommand(agentCmd.NewCmdAgent(f))
	cmd.AddCommand(aiCmd.NewCmdAI(f))
	cmd.AddCommand(buildCmd.NewCmdBuild(f))
	cmd.AddCommand(clusterCmd.NewCmdCluster(f))
	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(jobCmd.NewCmdJob(f))
	cmd.AddCommand(pipelineCmd.NewCmdPipeline(f))
	cmd.AddCommand(useCmd.NewCmdUse(f))
	cmd.AddCommand(versionCmd.NewCmdVersion(f))

	return cmd, nil
}
