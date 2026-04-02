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
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type DeleteCmd struct {
	TeamUUID string `arg:"" help:"UUID of the team to delete" name:"team-uuid"`
}

func (c *DeleteCmd) Help() string {
	return `
Delete a team from the organization.

You will be prompted to confirm deletion unless --yes is set.

Examples:
  # Delete a team (with confirmation prompt)
  $ bk team delete my-team-uuid

  # Delete a team without confirmation
  $ bk team delete my-team-uuid --yes
`
}

func (c *DeleteCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Are you sure you want to delete team %s?", c.TeamUUID))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Deletion cancelled.")
		return nil
	}

	spinErr := bkIO.SpinWhile(f, "Deleting team", func() {
		_, err = f.RestAPIClient.Teams.DeleteTeam(ctx, f.Config.OrganizationSlug(), c.TeamUUID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error deleting team: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Team deleted successfully.")
	return nil
}
