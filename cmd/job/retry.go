package job

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

const jobCommandPrefix = "JobTypeCommand---"

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

	// Given a job UUID argument, we need to generate the GraphQL ID matching
	graphqlID := util.GenerateGraphQLID(jobCommandPrefix, c.JobID)

	ctx := context.Background()
	var j *bkGraphQL.RetryJobResponse

	err = bkIO.SpinWhile(f, "Retrying job", func() {
		j, err = bkGraphQL.RetryJob(ctx, f.GraphQLClient, graphqlID)
	})
	if err != nil {
		return err
	}

	// Fixes segfault when error is returned, e.g. "Jobs from canceled builds cannot be retried"
	if j == nil || j.JobTypeCommandRetry == nil {
		return fmt.Errorf("failed to retry job")
	}

	fmt.Println("Successfully retried job: " + j.JobTypeCommandRetry.JobTypeCommand.Url)
	return nil
}
