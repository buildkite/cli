package job

import (
	"context"
	"fmt"
	"regexp"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type LogCmd struct {
	JobID        string `arg:"" help:"Job UUID to get logs for"`
	Pipeline     string `help:"Deprecated; ignored because job UUIDs are scoped to the selected organization" short:"p"`
	BuildNumber  string `help:"Deprecated; ignored because job UUIDs are scoped to the selected organization" short:"b"`
	NoTimestamps bool   `help:"Strip timestamp prefixes from log output" name:"no-timestamps"`
}

func (c *LogCmd) Help() string {
	return `
Examples:
  # Get a job's logs by UUID
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31

  # Strip timestamp prefixes from output
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 --no-timestamps
`
}

func (c *LogCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	var logContent string
	if err = bkIO.SpinWhile(f, "Fetching job log", func() error {
		jobLog, apiErr := getJobLog(
			ctx,
			f.RestAPIClient,
			f.Config.OrganizationSlug(),
			c.JobID,
		)
		if apiErr != nil {
			return apiErr
		}
		logContent = jobLog.Content
		return nil
	}); err != nil {
		return err
	}

	if c.NoTimestamps {
		logContent = stripTimestamps(logContent)
	}

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	fmt.Fprint(writer, logContent)
	return nil
}

var timestampRegex = regexp.MustCompile(`bk;t=\d+\x07`)

func stripTimestamps(content string) string {
	return timestampRegex.ReplaceAllString(content, "")
}
