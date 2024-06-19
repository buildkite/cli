package cluster

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/cluster"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdClusterView(f *factory.Factory) *cobra.Command {
	var clusterID string
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view  [flags]",
		Args:                  cobra.MinimumNArgs(0),
		Short:                 "View cluster information.",
		Long: heredoc.Doc(`
			View cluster information.

			It accepts org slug and cluster id.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {

			orgSlug := f.Config.OrganizationSlug()

			l := io.NewPendingCommand(func() tea.Msg {

				summary, err := cluster.ClusterSummary(cmd.Context(), orgSlug, clusterID, f)

				if err != nil {
					return err
				}

				return io.PendingOutput(summary)
			}, "Loading cluster information")

			p := tea.NewProgram(l)
			_, err := p.Run()

			return err
		},
	}

	cmd.Flags().StringVarP(&clusterID, "clusterID", "c", "", "ID of the cluster")
	cmd.Flags().SortFlags = false

	return &cmd

}
