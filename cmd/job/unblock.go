package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type UnblockCmd struct {
	JobID string `arg:"" help:"Job UUID to unblock"`
	Data  string `help:"JSON formatted data to unblock the job"`
}

func (c *UnblockCmd) Help() string {
	return `
Unblock a job.

Use this command to unblock build jobs.

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

	fields, err := c.unblockFields()
	if err != nil {
		return err
	}

	var job buildkite.Job
	err = bkIO.SpinWhile(f, "Unblocking job", func() error {
		var apiErr error
		job, apiErr = unblockJob(ctx, f.RestAPIClient, organization, c.JobID, fields)
		return apiErr
	})
	if err != nil {
		if isAlreadyUnblocked(err) {
			fmt.Println("This job is already unblocked")
			return nil
		}
		return err
	}

	if job.WebURL != "" {
		fmt.Println("Successfully unblocked job: " + job.WebURL)
		return nil
	}

	fmt.Println("Successfully unblocked job")
	return nil
}

func (c *UnblockCmd) unblockFields() (map[string]any, error) {
	if bkIO.HasDataAvailable(os.Stdin) {
		stdin := new(strings.Builder)
		if _, err := io.Copy(stdin, os.Stdin); err != nil {
			return nil, err
		}
		return parseUnblockFields(stdin.String())
	} else if c.Data != "" {
		return parseUnblockFields(c.Data)
	}

	return nil, nil
}

func parseUnblockFields(input string) (map[string]any, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(input), &fields); err != nil {
		return nil, fmt.Errorf("parsing unblock data as JSON: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("unblock data must be a JSON object")
	}

	return fields, nil
}

func isAlreadyUnblocked(err error) bool {
	var apiErr *buildkite.ErrorResponse
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Message == "The job's state must be blocked" || apiErr.Message == "The job's state must be blocked."
}
