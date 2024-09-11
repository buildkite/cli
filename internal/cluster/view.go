package cluster

import (
	"bytes"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func ClusterViewTable(c ...Cluster) string {
	tableOut := &bytes.Buffer{}

	t := table.New().Headers("Name", "ID", "Queues").Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().PaddingRight(2)
	})

	if len(c) == 1 {
		t.Row(c[0].Name, c[0].ID, parseQueuesAsString(c[0].Queues...))
	} else {
		for _, cl := range c {
			t.Row(cl.Name, cl.ID, parseQueuesAsString(cl.Queues...))
		}
	}

	fmt.Fprint(tableOut, t.Render())
	return tableOut.String()
}

func parseQueuesAsString(queues ...ClusterQueue) string {
	queuesString := ""
	for _, q := range queues {
		queuesString = queuesString + q.Key + ", "
	}
	return queuesString[:len(queuesString)-2]
}
