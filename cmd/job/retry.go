package job

import (
	"context"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type RetryCmd struct {
	JobID string `arg:"" help:"Job UUID to retry"`
	output.OutputFlags
}

func (c *RetryCmd) Help() string {
	return `Use this command to retry build jobs.
Structured output contains the new job returned by the API; use its id to follow the retry.

Examples:
  # Retry a job by UUID
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31

  # Get the new job UUID
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31 --json | jq -r '.id'

  # Print a human-readable confirmation
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31 --text
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	return output.WriteTextOrStructured(kongCtx.Stdout, format, job, "Successfully retried job: "+job.WebURL)
}
