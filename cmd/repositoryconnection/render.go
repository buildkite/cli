package repositoryconnection

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func commandWriter(f *factory.Factory, format output.Format) (io.Writer, func() error) {
	if format != output.FormatText {
		return os.Stdout, func() error { return nil }
	}
	return bkIO.Pager(f.NoPager, f.Config.Pager())
}

// jsonKeyed writes YAML with the same keys as JSON. The SDK models only have
// JSON tags, so encoding them directly as YAML would use lowercased Go field
// names such as displayname and emit omitted fields as null.
type jsonKeyed struct {
	value any
}

func (j jsonKeyed) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.value)
}

func (j jsonKeyed) MarshalYAML() (any, error) {
	encoded, err := json.Marshal(j.value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func renderConnectionText(connection buildkite.RepositoryConnection) string {
	serviceAccount := "-"
	if connection.ServiceAccount != nil {
		serviceAccount = output.ValueOrDash(connection.ServiceAccount.Login)
	}
	hostType, hostURL := "-", "-"
	if connection.Host != nil {
		hostType = output.ValueOrDash(connection.Host.Type)
		hostURL = output.ValueOrDash(connection.Host.URL)
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Repository Connection: %s\n\n", output.ValueOrDash(connection.DisplayName))
	result.WriteString(output.Table(
		[]string{"Field", "Value"},
		[][]string{
			{"Display Name", output.ValueOrDash(connection.DisplayName)},
			{"Type", output.ValueOrDash(connection.Type)},
			{"ID", output.ValueOrDash(connection.ID)},
			{"Service Account", serviceAccount},
			{"Host Type", hostType},
			{"Host URL", hostURL},
			{"API URL", output.ValueOrDash(connection.URL)},
		},
		map[string]string{"field": "dim", "value": "italic"},
	))

	result.WriteString("\nGitHub API rate limit (GitHub installation quota, not the Buildkite API rate limit):\n\n")
	rateLimit := connection.RateLimit
	if rateLimit == nil {
		result.WriteString("Not available: no cached quota, or this connection type has no GitHub quota.\n")
		return result.String()
	}

	resetAt := rateLimit.ResetAt.Format(time.RFC3339)
	if rateLimit.ResetAt.Before(time.Now()) {
		resetAt += " (passed; used and remaining are stale)"
	}
	result.WriteString(output.Table(
		[]string{"Field", "Value"},
		[][]string{
			{"Limit", strconv.FormatInt(rateLimit.Limit, 10)},
			{"Used", strconv.FormatInt(rateLimit.Used, 10)},
			{"Remaining", strconv.FormatInt(rateLimit.Remaining, 10)},
			{"Reset At", resetAt},
		},
		map[string]string{"field": "dim", "value": "italic"},
	))
	return result.String()
}
