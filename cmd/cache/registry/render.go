package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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

func renderRegistryText(registry buildkite.CacheRegistry) string {
	rows := [][]string{
		{"Slug", output.ValueOrDash(registry.Slug)},
		{"Name", output.ValueOrDash(registry.Name)},
		{"Description", optionalString(registry.Description)},
		{"Emoji", optionalString(registry.Emoji)},
		{"Color", optionalString(registry.Color)},
		{"UUID", output.ValueOrDash(registry.UUID)},
		{"API URL", output.ValueOrDash(registry.URL)},
		{"Cluster URL", output.ValueOrDash(registry.ClusterURL)},
	}
	if registry.CreatedAt != nil {
		rows = append(rows, []string{"Created At", registry.CreatedAt.Format(time.RFC3339)})
	}
	if registry.UpdatedAt != nil {
		rows = append(rows, []string{"Updated At", registry.UpdatedAt.Format(time.RFC3339)})
	}
	if registry.Policy != nil {
		policy, err := json.MarshalIndent(registry.Policy, "", "  ")
		if err == nil {
			rows = append(rows, []string{"Policy", string(policy)})
		}
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Cache Registry: %s\n\n", output.ValueOrDash(registry.Slug))
	result.WriteString(output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	))
	return result.String()
}

func optionalString(value *string) string {
	if value == nil {
		return "-"
	}
	return output.ValueOrDash(*value)
}
