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

type ReprioritizeCmd struct {
	JobID       string `arg:"" help:"Job UUID to reprioritize"`
	Priority    int    `arg:"" help:"New priority value for the job"`
	Pipeline    string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"p"`
	BuildNumber string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"b"`
}

func (c *ReprioritizeCmd) Help() string {
	return `
Examples:
  # Reprioritize a job by UUID
  $ bk job reprioritize 0190046e-e199-453b-a302-a21a4d649d31 1
`
}

func (c *ReprioritizeCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
	warnIgnoredJobContextFlags(kongCtx.Stderr, c.Pipeline, c.BuildNumber)

	ctx := context.Background()

	var job buildkite.Job
	if err = bkIO.SpinWhile(f, "Reprioritizing job", func() error {
		var apiErr error
		job, apiErr = reprioritizeJob(
			ctx,
			f.RestAPIClient,
			organization,
			c.JobID,
			c.Priority,
		)
		return apiErr
	}); err != nil {
		return err
	}

	fmt.Printf("Job reprioritized to %d: %s\n", c.Priority, job.WebURL)
	return nil
}
