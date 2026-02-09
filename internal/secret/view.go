package secret

import (
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// SecretViewTable renders a table view of one or more cluster secrets
func SecretViewTable(secrets ...buildkite.ClusterSecret) string {
	if len(secrets) == 0 {
		return "No secrets found."
	}

	rows := make([][]string, 0, len(secrets))
	for _, s := range secrets {
		rows = append(rows, []string{
			output.ValueOrDash(s.Key),
			output.ValueOrDash(s.ID),
			output.ValueOrDash(s.Description),
		})
	}

	return output.Table(
		[]string{"Key", "ID", "Description"},
		rows,
		map[string]string{"key": "bold", "id": "dim", "description": "dim"},
	)
}
