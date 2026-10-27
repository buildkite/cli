package job

import (
	"context"
	"fmt"
	"sort"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type EnvironmentCmd struct {
	JobID string `arg:"" help:"Job UUID to read environment variables for"`
	output.OutputFlags
}

func (c *EnvironmentCmd) Help() string {
	return `Read a job's environment using its UUID and the selected organization.
Requires a token with the read_job_env scope.

Values may contain secrets. Output is not redacted: avoid CI logs, shared files,
or sending it to an LLM without reviewing it. Debug output omits response bodies.
JSON/YAML contain {"env": {"NAME": "value"}}; an empty environment is {"env": {}}.
Text prints sorted NAME="value" lines with escaped values, not a shell script.

Examples:
  $ bk job environment 0190046e-e199-453b-a302-a21a4d649d31 --json
  $ bk job environment 0190046e-e199-453b-a302-a21a4d649d31 --json | jq -r '.env.CI'
  $ bk job environment 0190046e-e199-453b-a302-a21a4d649d31 --text
`
}

func (c *EnvironmentCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	f.Quiet = globals.IsQuiet()

	organization, err := configuredOrganization(f.Config.OrganizationSlug())
	if err != nil {
		return err
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	var environment buildkite.JobEnvs
	if err := bkIO.SpinWhile(f, "Fetching job environment", func() error {
		var apiErr error
		environment, apiErr = getJobEnvironment(context.Background(), f.RestAPIClient, organization, c.JobID)
		return apiErr
	}); err != nil {
		return err
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	if format != output.FormatText {
		return output.Write(kongCtx.Stdout, map[string]map[string]string{"env": environment.EnvironmentVariables}, format)
	}
	keys := make([]string, 0, len(environment.EnvironmentVariables))
	for key := range environment.EnvironmentVariables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(kongCtx.Stdout, "%s=%q\n", key, environment.EnvironmentVariables[key]); err != nil {
			return err
		}
	}
	return nil
}
