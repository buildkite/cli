package repositoryconnection

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type ListCmd struct {
	output.OutputFlags
}

func (c *ListCmd) Help() string {
	return `
List the organization's source control repository connections, such as GitHub
apps, Bitbucket Server, and GitLab Self-Managed. Requires organization
administrator access.

The list does not include GitHub API rate limits; use
"bk repository-connection view" to see a connection's rate limit.

Examples:
  # List repository connections
  $ bk repository-connection list

  # List repository connections in JSON format
  $ bk repository-connection list -o json
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

func (c *ListCmd) run(ctx context.Context, f *factory.Factory, org string, writer io.Writer, format output.Format) error {
	var connections []buildkite.RepositoryConnection
	if err := bkIO.SpinWhile(f, "Loading repository connections", func() error {
		var apiErr error
		connections, _, apiErr = f.RestAPIClient.RepositoryConnections.List(ctx, org)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error fetching repository connections: %w", err)
	}
	if connections == nil {
		connections = []buildkite.RepositoryConnection{}
	}

	if format != output.FormatText {
		return output.Write(writer, jsonKeyed{connections}, format)
	}
	if len(connections) == 0 {
		_, err := fmt.Fprintln(writer, "No repository connections found")
		return err
	}

	rows := make([][]string, 0, len(connections))
	for _, connection := range connections {
		rows = append(rows, []string{
			output.ValueOrDash(connection.DisplayName),
			output.ValueOrDash(connection.Type),
			output.ValueOrDash(connection.ID),
		})
	}
	var result strings.Builder
	fmt.Fprintf(&result, "Repository Connections (%d) in %s\n\n%s\n", len(connections), org, output.Table(
		[]string{"Display Name", "Type", "ID"},
		rows,
		map[string]string{"display name": "bold", "type": "dim"},
	))
	result.WriteString("Run `bk repository-connection view <id>` to see a connection's GitHub API rate limit.\n")
	_, err := io.WriteString(writer, result.String())
	return err
}
