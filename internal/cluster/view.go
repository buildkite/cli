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

	//summary := ""
	if len(clusterSummary.ClusterID) > 0 {
		fmt.Fprint(tableOut, bold.Render("Cluster name: "+clusterSummary.Name, "\n"))
		fmt.Fprint(tableOut, bold.Render("\nCluster Description: "+clusterSummary.Description, "\n"))
		//summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("Cluster ID: ", clusterSummary.ClusterID, "\n")
		//summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("Orgnization: ", clusterSummary.OrganizationSlug, "\n")
		//summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("Queue Name \n")
		for _, queue := range clusterSummary.Queues {

			t.Row(queue.Name, strconv.Itoa(queue.Agent))
			//summary += lipgloss.NewStyle().Bold(false).Padding(0, 1).Render(queue.Name, "\n")
			//summary += lipgloss.NewStyle().Bold(false).Padding(0, 1).Render("Agents", strconv.Itoa(queue.Agent), "\n")
		}
	}

	fmt.Fprint(tableOut, t.Render())

	return tableOut.String(), nil
}
