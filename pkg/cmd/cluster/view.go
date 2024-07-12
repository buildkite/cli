package cluster

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/cluster"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

func NewCmdClusterView(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view  id ",
		Args:                  cobra.MinimumNArgs(1),
		Short:                 "View cluster information.",
		Long: heredoc.Doc(`
			View cluster information.

			It accepts org slug and cluster id.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgSlug := f.Config.OrganizationSlug()
			clusterID := args[0]

			var err error
			var output string
			spinErr := spinner.New().
				Title("Loading cluster information").
				Action(func() {
					output, err = cluster.ClusterSummary(cmd.Context(), orgSlug, clusterID, f)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", output)

			return err
		},
	}

	return &cmd

}
