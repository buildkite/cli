package team

import (
	"fmt"
	"strings"
	"time"

	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// TeamViewTable renders a table view of one or more teams
func TeamViewTable(t ...buildkite.Team) string {
	if len(t) == 0 {
		return "No teams found."
	}

	if len(t) == 1 {
		return renderSingleTeamDetail(t[0])
	}

	rows := make([][]string, 0, len(t))
	for _, team := range t {
		rows = append(rows, []string{
			output.ValueOrDash(team.Name),
			output.ValueOrDash(team.Slug),
			output.ValueOrDash(team.Privacy),
			fmt.Sprintf("%v", team.Default),
		})
	}

	return output.Table(
		[]string{"Name", "Slug", "Privacy", "Default"},
		rows,
		map[string]string{"name": "bold", "slug": "dim", "privacy": "dim", "default": "dim"},
	)
}

// RenderTeamText renders a single team as a human-readable text table.
func RenderTeamText(t buildkite.Team) string {
	return renderSingleTeamDetail(t)
}

func renderSingleTeamDetail(t buildkite.Team) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Viewing %s\n\n", output.ValueOrDash(t.Name))

	rows := [][]string{
		{"Name", output.ValueOrDash(t.Name)},
		{"Slug", output.ValueOrDash(t.Slug)},
		{"Description", output.ValueOrDash(t.Description)},
		{"Privacy", output.ValueOrDash(t.Privacy)},
		{"Default", fmt.Sprintf("%v", t.Default)},
		{"ID", output.ValueOrDash(t.ID)},
	}

	if t.CreatedBy != nil && t.CreatedBy.ID != "" {
		rows = append(rows,
			[]string{"Created By Name", output.ValueOrDash(t.CreatedBy.Name)},
			[]string{"Created By Email", output.ValueOrDash(t.CreatedBy.Email)},
			[]string{"Created By ID", output.ValueOrDash(t.CreatedBy.ID)},
		)
	}

	if t.CreatedAt != nil {
		rows = append(rows, []string{"Created At", t.CreatedAt.Format(time.RFC3339)})
	}

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)
	return sb.String()
}
