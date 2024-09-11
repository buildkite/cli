package cluster

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/cluster"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

func NewCmdClusterView(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view <id>",
		Args:                  cobra.MinimumNArgs(1),
		Short:                 "View cluster information.",
		Long: heredoc.Doc(`
			View cluster information.

			It accepts org slug and cluster id.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			orgSlug := f.Config.OrganizationSlug()
			clusterID := args[0]
			clusterRes, _, err := f.RestAPIClient.Clusters.Get(orgSlug, clusterID)

			if err != nil {
				return err
			}

			var output string
			spinErr := spinner.New().
				Title("Loading cluster information").
				Action(func() {
					queues, _ := cluster.GetQueues(cmd.Context(), f, orgSlug, *clusterRes.ID, &buildkite.ClusterQueuesListOptions{})
					output = cluster.ClusterViewTable(cluster.Cluster{
						Name:           *clusterRes.Name,
						ID:             *clusterRes.ID,
						DefaultQueueID: clusterRes.DefaultQueueID,
						Queues:         queues,
						Color:          clusterRes.Color,
					})
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", output)

			return err
		},
	}

	return &cmd
}
