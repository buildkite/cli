package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const jobBlockPrefix = "JobTypeBlock---"

type UnblockCmd struct {
	JobID string `arg:"" help:"Job UUID to unblock"`
	Data  string `help:"JSON formatted data to unblock the job"`
}

func (c *UnblockCmd) Help() string {
	return `
Unblock a job.

Use this command to unblock build jobs.
Currently, this does not support submitting fields to the step.

Examples:
  # Unblock a job by UUID
  $ bk job unblock 0190046e-e199-453b-a302-a21a4d649d31

  # Unblock with JSON data
  $ bk job unblock 0190046e-e199-453b-a302-a21a4d649d31 --data '{"field": "value"}'

  # Unblock with data from stdin
  $ echo '{"field": "value"}' | bk job unblock 0190046e-e199-453b-a302-a21a4d649d31
`
}

func (c *UnblockCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
	graphqlID := util.GenerateGraphQLID(jobBlockPrefix, c.JobID)

	// Get unblock step fields if available
	var fields *string
	if bkIO.HasDataAvailable(os.Stdin) {
		stdin := new(strings.Builder)
		_, err := io.Copy(stdin, os.Stdin)
		if err != nil {
			return err
		}
		input := stdin.String()
		fields = &input
	} else if c.Data != "" {
		fields = &c.Data
	} else {
		// The GraphQL API errors if providing a null fields value so we need to provide an empty json object
		input := "{}"
		fields = &input
	}

	ctx := context.Background()
	err = bkIO.SpinWhile(f, "Unblocking job", func() {
		_, err = bkGraphQL.UnblockJob(ctx, f.GraphQLClient, graphqlID, fields)
	})
	if err != nil {
		// Handle a "graphql error" if the job is already unblocked
		var errList gqlerror.List
		if errors.As(err, &errList) {
			for _, gqlErr := range errList {
				if gqlErr.Message == "The job's state must be blocked" {
					fmt.Println("This job is already unblocked")
					return nil
				}
			}
		}
		return err
	}

	fmt.Println("Successfully unblocked job")
	return nil
}
