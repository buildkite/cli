package cluster

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func ClusterSummary(ctx context.Context, OrganizationSlug string, ClusterID string, f *factory.Factory) (string, error) {
	clusterSummary, err := QueryCluster(ctx, OrganizationSlug, ClusterID, f)
	if err != nil {
		return err.Error(), err
	}
	tableOut := &bytes.Buffer{}

	bold := lipgloss.NewStyle().Bold(true)

	t := table.New().Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().PaddingRight(2)
	}).Headers("Queues", "No of agents")

	if len(clusterSummary.ClusterID) > 0 {
		fmt.Fprint(tableOut, bold.Render("Cluster name: "+clusterSummary.Name, "\n"))
		fmt.Fprint(tableOut, bold.Render("\nCluster Description: "+clusterSummary.Description, "\n"))

		if len(clusterSummary.Queues) == 0 {
			fmt.Fprint(tableOut, "\n No Queues found for this cluster \n")
			return tableOut.String(), nil
		}
		for _, queue := range clusterSummary.Queues {
			t.Row(queue.Name, strconv.Itoa(queue.ActiveAgents))
		}
	}

	fmt.Fprint(tableOut, t.Render())

	return tableOut.String(), nil
}
