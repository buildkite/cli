package maintainer

import (
	"context"
	"fmt"
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
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CreateCmd struct {
	ClusterUUID string `arg:"" help:"Cluster UUID to add maintainer to" name:"cluster-uuid"`
	User        string `help:"User UUID to add as maintainer" optional:"" xor:"actor"`
	Team        string `help:"Team UUID to add as maintainer" optional:"" xor:"actor"`
	output.OutputFlags
}

func (c *CreateCmd) Help() string {
	return `
Create a cluster maintainer.

Either --user or --team must be specified.

Examples:
	# Create a user maintainer assignment
  $ bk maintainer create my-cluster-uuid --user user-uuid

	# Create a team maintainer assignment
  $ bk maintainer create my-cluster-uuid --team team-uuid
`
}

func (c *CreateCmd) Validate() error {
	if c.User == "" && c.Team == "" {
		return fmt.Errorf("either --user or --team must be specified")
	}
	return nil
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	input := buildkite.ClusterMaintainer{}
	if c.User != "" {
		input.UserID = c.User
	} else {
		input.TeamID = c.Team
	}

	var maintainer buildkite.ClusterMaintainerEntry
	if err = bkIO.SpinWhile(f, "Creating cluster maintainer", func() error {
		var apiErr error
		maintainer, _, apiErr = f.RestAPIClient.ClusterMaintainers.Create(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error creating cluster maintainer: %v", err)
	}

	maintainerView := output.Viewable[buildkite.ClusterMaintainerEntry]{
		Data:   maintainer,
		Render: renderMaintainerText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, maintainerView, format)
	}

	fmt.Fprintf(os.Stdout, "Maintainer created successfully\n\n")
	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()
	return output.Write(writer, maintainerView, format)
}

func renderMaintainerText(m buildkite.ClusterMaintainerEntry) string {
	name := m.Actor.Slug
	if m.Actor.Name != "" {
		name = m.Actor.Name
	}

	rows := [][]string{
		{"Assignment ID", output.ValueOrDash(m.ID)},
		{"Actor ID", output.ValueOrDash(m.Actor.ID)},
		{"Type", output.ValueOrDash(m.Actor.Type)},
		{"Name", output.ValueOrDash(name)},
	}

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Maintainer assignment %s\n\n", output.ValueOrDash(m.ID))
	sb.WriteString(table)

	return sb.String()
}
