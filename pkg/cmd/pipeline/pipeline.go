package pipeline

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdPipeline(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "pipeline <command>",
		Short: "Manage pipelines",
		Long:  "Work with Buildkite pipelines.",
		Example: heredoc.Doc(`
			# To create a new pipeline
			$ bk pipeline create my-org/my-pipeline
			
			# To validate a pipeline configuration
			$ bk pipeline validate
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}

	cmd.AddCommand(NewCmdPipelineCreate(f))
	cmd.AddCommand(NewCmdPipelineView(f))
	cmd.AddCommand(NewCmdPipelineValidate(f))

	return &cmd
}
