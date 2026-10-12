package job

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type CancelCmd struct {
	JobID string `arg:"" help:"Job UUID to cancel" required:""`
	Web   bool   `help:"Open the job in a web browser after it has been cancelled" short:"w"`
}

func (c *CancelCmd) Help() string {
	return `
Other jobs in the build keep running; to cancel the whole build, use
bk build cancel. A running job stays canceling until its agent stops it.

Examples:
  # Cancel a job (with confirmation prompt)
  $ bk job cancel 0190046e-e199-453b-a302-a21a4d649d31

  # Cancel a job without confirmation (useful for automation)
  $ bk job --yes cancel 0190046e-e199-453b-a302-a21a4d649d31

  # Cancel a job and open it in browser
  $ bk job --yes cancel 0190046e-e199-453b-a302-a21a4d649d31 --web
`
}

func (c *CancelCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	organization, err := configuredOrganization(f.Config.OrganizationSlug())
	if err != nil {
		return err
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Cancel job %s", c.JobID))
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	var job buildkite.Job
	if err := bkIO.SpinWhile(f, fmt.Sprintf("Cancelling job %s", c.JobID), func() error {
		var apiErr error
		job, apiErr = cancelJob(context.Background(), f.RestAPIClient, organization, c.JobID)
		return apiErr
	}); err != nil {
		return err
	}

	// Running jobs stay "canceling" until their agent stops them.
	if job.State == "canceling" {
		fmt.Fprintf(kongCtx.Stdout, "Job canceling: %s\n", job.WebURL)
	} else {
		fmt.Fprintf(kongCtx.Stdout, "Job canceled: %s\n", job.WebURL)
	}

	return util.OpenInWebBrowser(c.Web, job.WebURL)
}
