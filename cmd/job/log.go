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
	"github.com/mcncl/terminal-to-llm/digest"
)

type LogCmd struct {
	JobID        string `arg:"" help:"Job UUID to get logs for"`
	Pipeline     string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"p"`
	BuildNumber  string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"b"`
	NoTimestamps bool   `help:"Strip timestamp prefixes from log output" name:"no-timestamps"`
	LLMOptimized bool   `help:"Format output to be optimal for LLM consumption (strips ANSI, deduplicates loops)" name:"agent"`
	Format       string `help:"Output rendering for --agent: plain or markdown" name:"format" enum:"plain,markdown" default:"plain"`
	MaxTokens    int    `help:"Hard ceiling on the estimated token count of --agent output (0 = unlimited)" name:"max-tokens"`
	NoWindow     bool   `help:"Disable failure-focused windowing in --agent output (keep all lines)" name:"no-window"`
}

func (c *LogCmd) Help() string {
	return `
Examples:
  # Get a job's logs by UUID
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31

  # Strip timestamp prefixes from output
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 --no-timestamps

  # Format for LLM consumption
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 --agent

  # Format for LLM as markdown, capped at 2000 tokens, keeping all lines
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 --agent --format markdown --max-tokens 2000 --no-window
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

	organization, err := configuredOrganization(f.Config.OrganizationSlug())
	if err != nil {
		return err
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}
	warnIgnoredJobContextFlags(kongCtx.Stderr, c.Pipeline, c.BuildNumber)

	ctx := context.Background()

	var logContent string
	if err = bkIO.SpinWhile(f, "Fetching job log", func() error {
		jobLog, apiErr := getJobLog(
			ctx,
			f.RestAPIClient,
			organization,
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

	if c.LLMOptimized {
		opt := digest.Default()
		opt.Format = digest.ParseFormat(c.Format)
		opt.MaxTokens = c.MaxTokens
		opt.Window = !c.NoWindow
		logContent = digest.Process([]byte(logContent), opt)
	}

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	fmt.Fprint(writer, logContent)
	return nil
}

// timestampRegex matches Buildkite's inline timestamp markers, including the
// optional APC introducer (`\x1b_`) so the whole sequence is removed rather than
// leaving a dangling escape byte behind.
var timestampRegex = regexp.MustCompile(`(?:\x1b_)?bk;t=\d+\x07`)

func stripTimestamps(content string) string {
	return timestampRegex.ReplaceAllString(content, "")
}
