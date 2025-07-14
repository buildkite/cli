package cli

import "github.com/buildkite/cli/v3/pkg/cmd/factory"

// OutputFlag provides the --output flag for commands that support structured output formats
type OutputFlag struct {
	Output string `short:"o" help:"Output format (json, yaml, table, raw)" default:"table" enum:"json,yaml,table,raw"`
}

// Apply copies the chosen format into the factory so the existing output helpers
// (Print, ShouldUseStructuredOutput, etc.) continue to work unchanged.
func (o *OutputFlag) Apply(f *factory.Factory) {
	if o == nil {
		return
	}
	f.Output = o.Output
}
