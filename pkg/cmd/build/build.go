package build

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdBuild(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "build <command>",
		Short: "Manage pipeline builds",
		Long:  "Work with Buildkite pipeline builds.",
		Example: heredoc.Doc(`
			# To create a new build
			$ bk build new -m "Build from cli" -c "HEAD" -b "main"
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A pipeline is passed as an argument. It can be supplied in any of the following formats:
				- "PIPELINE_SLUG"
				- "ORGANIZATION_SLUG/PIPELINE_SLUG"
			`),
		},
	}

	cmd.AddCommand(NewCmdBuildCancel(f))
	cmd.AddCommand(NewCmdBuildDownload(f))
	cmd.AddCommand(NewCmdBuildNew(f))
	cmd.AddCommand(NewCmdBuildRebuild(f))
	cmd.AddCommand(NewCmdBuildView(f))
	cmd.AddCommand(NewCmdBuildWatch(f))

	return &cmd
}

func renderResult(result string) string {
	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Padding(0, 0).Render(result))
}
