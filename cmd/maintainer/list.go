package maintainer

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ListCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID to list maintainers for" name:"cluster-uuid"`
	output.OutputFlags
}

func (c *ListCmd) Help() string {
	return `
List cluster maintainers.

Examples:
  # List all maintainers for a cluster
  $ bk maintainer list my-cluster-uuid

  # List in JSON format
  $ bk maintainer list my-cluster-uuid -o json
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var maintainers []buildkite.ClusterMaintainerEntry
	if err = bkIO.SpinWhile(f, "Fetching cluster maintainers", func() error {
		var apiErr error
		maintainers, _, apiErr = f.RestAPIClient.ClusterMaintainers.List(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, nil)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error fetching cluster maintainers: %v", err)
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, maintainers, format)
	}

	if len(maintainers) == 0 {
		fmt.Fprintln(os.Stdout, "No maintainers found")
		return nil
	}

	rows := make([][]string, 0, len(maintainers))
	for _, m := range maintainers {
		name := m.Actor.Slug
		if m.Actor.Name != "" {
			name = m.Actor.Name
		}

		rows = append(rows, []string{m.ID, m.Actor.Type, name})
	}

	table := output.Table(
		[]string{"ID", "Type", "Name"},
		rows,
		map[string]string{"id": "bold", "name": "italic"},
	)

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	_, err = fmt.Fprintf(writer, "Maintainers (%d)\n\n%s\n", len(maintainers), table)
	return err
}
