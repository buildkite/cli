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

type UpdateCmd struct {
	TeamUUID                  string `arg:"" help:"UUID of the team to update" name:"team-uuid"`
	Name                      string `help:"New name for the team" optional:""`
	Description               string `help:"New description for the team" optional:""`
	Privacy                   string `help:"Privacy setting: visible or secret" optional:""`
	Default                   *bool  `help:"Whether this is the default team for new members" optional:"" name:"default"`
	DefaultMemberRole         string `help:"Default role for new members: member or maintainer" optional:"" name:"default-member-role"`
	MembersCanCreatePipelines *bool  `help:"Whether members can create pipelines" optional:"" name:"members-can-create-pipelines"`
	output.OutputFlags
}

func (c *UpdateCmd) Help() string {
	return `
Update an existing team's settings.

Examples:
  # Rename a team
  $ bk team update my-team-uuid --name "New Team Name"

  # Change a team's privacy
  $ bk team update my-team-uuid --privacy secret

  # Update description and default member role
  $ bk team update my-team-uuid --description "Updated description" --default-member-role maintainer
`
}

func (c *UpdateCmd) Validate() error {
	if c.Name == "" && c.Description == "" && c.Privacy == "" && c.Default == nil && c.DefaultMemberRole == "" && c.MembersCanCreatePipelines == nil {
		return fmt.Errorf("at least one of --name, --description, --privacy, --default, --default-member-role, or --members-can-create-pipelines must be provided")
	}
	if c.Privacy != "" && c.Privacy != "visible" && c.Privacy != "secret" {
		return fmt.Errorf("--privacy must be either \"visible\" or \"secret\"")
	}
	if c.DefaultMemberRole != "" && c.DefaultMemberRole != "member" && c.DefaultMemberRole != "maintainer" {
		return fmt.Errorf("--default-member-role must be either \"member\" or \"maintainer\"")
	}
	return nil
}

func (c *UpdateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	// Fetch current state to use as base for update
	var current buildkite.Team
	spinErr := bkIO.SpinWhile(f, "Loading team", func() {
		current, err = f.RestAPIClient.Teams.GetTeam(ctx, f.Config.OrganizationSlug(), c.TeamUUID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error fetching team: %v", err)
	}

	// Build update input from current values, overriding with any flags set
	input := buildkite.CreateTeam{
		Name:                      current.Name,
		Description:               current.Description,
		Privacy:                   current.Privacy,
		IsDefaultTeam:             current.Default,
		MembersCanCreatePipelines: false,
	}
	if c.Name != "" {
		input.Name = c.Name
	}
	if c.Description != "" {
		input.Description = c.Description
	}
	if c.Privacy != "" {
		input.Privacy = c.Privacy
	}
	if c.Default != nil {
		input.IsDefaultTeam = *c.Default
	}
	if c.DefaultMemberRole != "" {
		input.DefaultMemberRole = c.DefaultMemberRole
	}
	if c.MembersCanCreatePipelines != nil {
		input.MembersCanCreatePipelines = *c.MembersCanCreatePipelines
	}

	var t buildkite.Team
	spinErr = bkIO.SpinWhile(f, "Updating team", func() {
		t, _, err = f.RestAPIClient.Teams.UpdateTeam(ctx, f.Config.OrganizationSlug(), c.TeamUUID, input)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error updating team: %v", err)
	}

	teamView := output.Viewable[buildkite.Team]{
		Data:   t,
		Render: team.RenderTeamText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, teamView, format)
	}

	fmt.Fprintln(os.Stderr, "Team updated successfully.")
	fmt.Fprintln(os.Stdout)
	return output.Write(os.Stdout, teamView, format)
}
