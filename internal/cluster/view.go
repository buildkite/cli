package cluster

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// ClusterViewTable renders a table view of one or more clusters
func ClusterViewTable(c ...buildkite.Cluster) string {
	if len(c) == 0 {
		return "No clusters found."
	}

	if len(c) == 1 {
		return renderSingleClusterDetail(c[0])
	}

	rows := make([][]string, 0, len(c))
	for _, cluster := range c {
		rows = append(rows, []string{
			output.ValueOrDash(cluster.Name),
			output.ValueOrDash(cluster.ID),
			output.ValueOrDash(cluster.DefaultQueueID),
		})
	}

	return output.Table(
		[]string{"Name", "ID", "Default Queue ID"},
		rows,
		map[string]string{"name": "bold", "id": "dim", "default queue id": "dim"},
	)
}

func renderSingleClusterDetail(c buildkite.Cluster) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Viewing %s\n\n", output.ValueOrDash(c.Name))

	rows := [][]string{
		{"Description", output.ValueOrDash(c.Description)},
		{"Color", output.ValueOrDash(c.Color)},
		{"Emoji", output.ValueOrDash(c.Emoji)},
		{"ID", output.ValueOrDash(c.ID)},
		{"GraphQL ID", output.ValueOrDash(c.GraphQLID)},
		{"Default Queue ID", output.ValueOrDash(c.DefaultQueueID)},
		{"Web URL", output.ValueOrDash(c.WebURL)},
		{"API URL", output.ValueOrDash(c.URL)},
		{"Queues URL", output.ValueOrDash(c.QueuesURL)},
		{"Queue URL", output.ValueOrDash(c.DefaultQueueURL)},
	}

	if c.CreatedBy.ID != "" {
		rows = append(rows,
			[]string{"Created By Name", output.ValueOrDash(c.CreatedBy.Name)},
			[]string{"Created By Email", output.ValueOrDash(c.CreatedBy.Email)},
			[]string{"Created By ID", output.ValueOrDash(c.CreatedBy.ID)},
		)
	}

	if c.CreatedAt != nil {
		rows = append(rows, []string{"Created At", c.CreatedAt.Format(time.RFC3339)})
	}

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)
	return sb.String()
}
