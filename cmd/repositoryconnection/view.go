package repositoryconnection

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type ViewCmd struct {
	ConnectionID string `arg:"" help:"Repository connection UUID, from bk repository-connection list" name:"connection-id"`
	output.OutputFlags
}

func (c *ViewCmd) Help() string {
	return `
View a repository connection's service account, host, and GitHub API rate limit.
Requires organization administrator access.

The rate limit is the cached GitHub installation core API quota for this
connection. It is not the Buildkite API rate limit. It is shown as unavailable
(rate_limit: null in JSON and YAML) when no cached quota exists or the
connection type has no GitHub quota. If the reset time has passed, the quota has
reset since it was cached, so the used and remaining values are stale.

Examples:
  # View a repository connection
  $ bk repository-connection view 01234567-89ab-cdef-0123-456789abcdef

  # View a repository connection in JSON format
  $ bk repository-connection view 01234567-89ab-cdef-0123-456789abcdef -o json
`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	writer, cleanup := commandWriter(f, format)
	defer func() { _ = cleanup() }()
	return c.run(ctx, f, f.Config.OrganizationSlug(), writer, format)
}

func (c *ViewCmd) run(ctx context.Context, f *factory.Factory, org string, writer io.Writer, format output.Format) error {
	var connection buildkite.RepositoryConnection
	if err := bkIO.SpinWhile(f, "Loading repository connection", func() error {
		var apiErr error
		connection, _, apiErr = f.RestAPIClient.RepositoryConnections.Get(ctx, org, c.ConnectionID)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error fetching repository connection: %w", err)
	}

	return output.Write(writer, connectionView{connection}, format)
}

// connectionView always includes service_account, host, and rate_limit in
// JSON and YAML output, so an unavailable value is an explicit null rather
// than a missing key.
type connectionView struct {
	connection buildkite.RepositoryConnection
}

func (v connectionView) TextOutput() string {
	return renderConnectionText(v.connection)
}

func (v connectionView) fields() (map[string]any, error) {
	encoded, err := json.Marshal(v.connection)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for _, key := range []string{"service_account", "host", "rate_limit"} {
		if _, ok := fields[key]; !ok {
			fields[key] = nil
		}
	}
	return fields, nil
}

func (v connectionView) MarshalJSON() ([]byte, error) {
	fields, err := v.fields()
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (v connectionView) MarshalYAML() (any, error) {
	return v.fields()
}
