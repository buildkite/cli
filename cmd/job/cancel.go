package job

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type CancelCmd struct {
	JobID string `arg:"" help:"Job ID to cancel" required:""`
	Web   bool   `help:"Open the job in a web browser after it has been cancelled" short:"w"`
}

func (c *CancelCmd) Help() string {
	return `
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
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()
	graphqlID := util.GenerateGraphQLID("JobTypeCommand---", c.JobID)

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Cancel job %s", c.JobID))
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	return c.cancelJob(ctx, c.JobID, graphqlID, f)
}

func (c *CancelCmd) cancelJob(ctx context.Context, displayID, apiID string, f *factory.Factory) error {
	var err error
	var result *graphql.CancelJobResponse
	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Cancelling job %s", displayID), func() {
		result, err = graphql.CancelJob(ctx, f.GraphQLClient, apiID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	job := result.JobTypeCommandCancel.JobTypeCommand
	fmt.Printf("Job canceled: %s\n", job.Url)

	return util.OpenInWebBrowser(c.Web, job.Url)
}
