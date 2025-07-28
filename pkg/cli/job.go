package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/buildkite/cli/v3/internal/graphql"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/factory"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Job commands
type JobCmd struct {
	Logs    JobLogsCmd    `cmd:"" help:"View job logs"`
	Retry   JobRetryCmd   `cmd:"" help:"Retry a job"`
	Unblock JobUnblockCmd `cmd:"" help:"Unblock a job"`
}

type JobLogsCmd struct {
	Job string `arg:"" help:"Job UUID to view logs for"`
}

func (j *JobLogsCmd) Help() string {
	return `Displays the complete log output for a job at the current point in time (does not stream or follow like tail -f).

EXAMPLES:
  # View logs for a specific job
  bk job logs 01234567-89ab-cdef-0123-456789abcdef

  # Get logs as JSON with metadata
  bk job logs 01234567-89ab-cdef-0123-456789abcdef --output json`
}

type JobRetryCmd struct {
	Job string `arg:"" help:"Job UUID to retry"`
}

type JobUnblockCmd struct {
	Job    string            `arg:"" help:"Job UUID to unblock"`
	Fields map[string]string `help:"Unblock form fields"`
}

func (j *JobUnblockCmd) Help() string {
	return `EXAMPLES:
  # Unblock a job
  bk job unblock 01234567-89ab-cdef-0123-456789abcdef

  # Unblock a job with form field data
  bk job unblock 01234567-89ab-cdef-0123-456789abcdef --fields "release_name=v1.2.0;environment=production"`
}

func (j *JobRetryCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Validate configuration
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Generate GraphQL ID for the job
	graphqlID := util.GenerateGraphQLID("JobTypeCommand---", j.Job)

	// Retry the job using GraphQL
	var err error
	var result *graphql.RetryJobResponse
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Retrying job %s", j.Job), func() {
		result, err = graphql.RetryJob(ctx, f.GraphQLClient, graphqlID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error retrying job: %w", err)
	}

	fmt.Printf("Job %s retried successfully: %s\n", j.Job, result.JobTypeCommandRetry.JobTypeCommand.Url)
	return nil
}

func (j *JobUnblockCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Validate configuration
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Generate GraphQL ID
	graphqlID := util.GenerateGraphQLID("JobTypeBlock---", j.Job)

	// Get unblock step fields if available
	var fields *string
	if bk_io.HasDataAvailable(os.Stdin) {
		stdin := new(strings.Builder)
		_, err := io.Copy(stdin, os.Stdin)
		if err != nil {
			return err
		}
		input := stdin.String()
		fields = &input
	} else {
		// Check if fields were provided via the Fields map
		if len(j.Fields) > 0 {
			fieldsJSON, err := json.Marshal(j.Fields)
			if err != nil {
				return fmt.Errorf("error marshaling fields: %w", err)
			}
			fieldsStr := string(fieldsJSON)
			fields = &fieldsStr
		} else {
			// the graphql API errors if providing a null fields value so we need to provide an empty json object
			input := "{}"
			fields = &input
		}
	}

	var err error
	spinErr := bk_io.SpinWhile("Unblocking job", func() {
		_, err = graphql.UnblockJob(ctx, f.GraphQLClient, graphqlID, fields)
	})
	if spinErr != nil {
		return spinErr
	}

	// handle a "graphql error" if the job is already unblocked
	if err != nil {
		var errList gqlerror.List
		if errors.As(err, &errList) {
			for _, err := range errList {
				if err.Message == "The job's state must be blocked" {
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
