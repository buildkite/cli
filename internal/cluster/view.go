package cluster

import (
	"github.com/buildkite/cli/v3/internal/ui"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// ClusterViewTable renders a table view of one or more clusters
func ClusterViewTable(c ...buildkite.Cluster) string {
	if len(c) == 0 {
		return "No clusters found."
	}

	if len(c) == 1 {
		// Render a detailed view for a single cluster
		return renderSingleClusterDetail(c[0])
	}

	// Render a summary table for multiple clusters
	var rows [][]string
	for _, cluster := range c {
		rows = append(rows, []string{
			cluster.Name,
			cluster.ID,
			cluster.DefaultQueueID,
		})
	}

	return ui.Table(rows, ui.WithHeaders("Name", "ID", "Default Queue ID"))
}

// renderSingleClusterDetail renders a detailed view of a single cluster
func renderSingleClusterDetail(c buildkite.Cluster) string {
	var sections []string

	// Basic Information
	basicInfo := []string{
		ui.LabeledValue("Name", c.Name),
	}
	if c.Emoji != "" {
		basicInfo = append(basicInfo, ui.LabeledValue("Emoji", c.Emoji))
	}
	if c.Description != "" {
		basicInfo = append(basicInfo, ui.LabeledValue("Description", c.Description))
	}
	if c.Color != "" {
		basicInfo = append(basicInfo, ui.LabeledValue("Color", c.Color))
	}
	sections = append(sections, ui.Section("Cluster Details", ui.SpacedVertical(basicInfo...)))

	// IDs and URLs
	idInfo := []string{
		ui.LabeledValue("ID", c.ID),
		ui.LabeledValue("GraphQL ID", c.GraphQLID),
		ui.LabeledValue("Default Queue ID", c.DefaultQueueID),
	}
	sections = append(sections, ui.Section("Identifiers", ui.SpacedVertical(idInfo...)))

	// URLs
	urlInfo := []string{
		ui.LabeledValue("Web URL", c.WebURL),
		ui.LabeledValue("API URL", c.URL),
		ui.LabeledValue("Queues URL", c.QueuesURL),
		ui.LabeledValue("Queue URL", c.DefaultQueueURL),
	}
	sections = append(sections, ui.Section("URLs", ui.SpacedVertical(urlInfo...)))

	// Creator Information
	if c.CreatedBy.ID != "" {
		creatorInfo := []string{
			ui.LabeledValue("Name", c.CreatedBy.Name),
			ui.LabeledValue("Email", c.CreatedBy.Email),
			ui.LabeledValue("ID", c.CreatedBy.ID),
			ui.LabeledValue("Created At", ui.FormatDate(c.CreatedAt.Time)),
		}
		sections = append(sections, ui.Section("Created By", ui.SpacedVertical(creatorInfo...)))
	}

	return ui.SpacedVertical(sections...)
}
