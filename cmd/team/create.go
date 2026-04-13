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

type CreateCmd struct {
	Name                      string `arg:"" help:"Name of the team" name:"name"`
	Description               string `help:"Description of the team" optional:""`
	Privacy                   string `help:"Privacy setting for the team: visible or secret" optional:"" default:"visible" enum:"visible,secret"`
	Default                   bool   `help:"Whether this is the default team for new members" optional:"" name:"default"`
	DefaultMemberRole         string `help:"Default role for new members: member or maintainer" optional:"" name:"default-member-role" default:"member" enum:"member,maintainer"`
	MembersCanCreatePipelines bool   `help:"Whether members can create pipelines" optional:"" name:"members-can-create-pipelines"`
	output.OutputFlags
}

func (c *CreateCmd) Help() string {
	return `
Create a new team in the organization.

Examples:
  # Create a team with default settings
  $ bk team create my-team

  # Create a private team with a description
  $ bk team create my-team --description "My team" --privacy secret

  # Create a default team where members can create pipelines
  $ bk team create my-team --default --members-can-create-pipelines
`
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

	input := buildkite.CreateTeam{
		Name:                      c.Name,
		Description:               c.Description,
		Privacy:                   c.Privacy,
		IsDefaultTeam:             c.Default,
		DefaultMemberRole:         c.DefaultMemberRole,
		MembersCanCreatePipelines: c.MembersCanCreatePipelines,
	}

	var t buildkite.Team
	spinErr := bkIO.SpinWhile(f, "Creating team", func() {
		t, _, err = f.RestAPIClient.Teams.CreateTeam(ctx, f.Config.OrganizationSlug(), input)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error creating team: %v", err)
	}

	teamView := output.Viewable[buildkite.Team]{
		Data:   t,
		Render: team.RenderTeamText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, teamView, format)
	}

	fmt.Fprintf(os.Stdout, "Team %s created successfully\n\n", t.Name)
	return output.Write(os.Stdout, teamView, format)
}
