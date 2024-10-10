package cluster

import (
	"bytes"
	"fmt"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func ClusterViewTable(c ...buildkite.Cluster) string {
	tableOut := &bytes.Buffer{}

	t := table.New().Headers("Name", "ID", "Default Queue ID").Border(lipgloss.HiddenBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().PaddingRight(2)
	})

	if len(c) == 1 {
		t.Row(*c[0].Name, *c[0].ID, *c[0].DefaultQueueID)
	} else {
		for _, cl := range c {
			t.Row(*cl.Name, *cl.ID, *cl.DefaultQueueID)
		}
	}

	fmt.Fprint(tableOut, t.Render())
	return tableOut.String()
}

// func parseQueuesAsString(queues ...buildkite.ClusterQueue) string {
// 	queuesString := ""
// 	for _, q := range queues {
// 		queuesString = queuesString + *q.Key + ", "
// 	}
// 	return queuesString[:len(queuesString)-2]
// }
