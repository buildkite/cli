package job

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type RetryCmd struct {
	JobID string `arg:"" help:"Job UUID to retry"`
}

func (c *RetryCmd) Help() string {
	return `Use this command to retry build jobs.

Examples:
  # Retry a job by UUID
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31
`
}

func (c *RetryCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	ctx := context.Background()

	var job buildkite.Job
	if err = bkIO.SpinWhile(f, "Retrying job", func() error {
		var apiErr error
		job, apiErr = retryJob(
			ctx,
			f.RestAPIClient,
			organization,
			c.JobID,
		)
		return apiErr
	}); err != nil {
		return err
	}

	fmt.Println("Successfully retried job: " + job.WebURL)
	return nil
}
