package team

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/team"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ViewCmd struct {
	TeamUUID string `arg:"" help:"UUID of the team to view" name:"team-uuid"`
	output.OutputFlags
}

func (c *ViewCmd) Help() string {
	return `
It accepts a team UUID.

Examples:
  # View a team
  $ bk team view my-team-uuid

  # View team in JSON format
  $ bk team view my-team-uuid -o json
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var t buildkite.Team
	spinErr := bkIO.SpinWhile(f, "Loading team information", func() {
		t, err = f.RestAPIClient.Teams.GetTeam(ctx, f.Config.OrganizationSlug(), c.TeamUUID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error fetching team: %v", err)
	}

	teamView := output.Viewable[buildkite.Team]{
		Data:   t,
		Render: team.RenderTeamText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, teamView, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	return output.Write(writer, teamView, format)
}
