package job

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type LogCmd struct {
	JobID        string `arg:"" help:"Job UUID to get logs for"`
	Pipeline     string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"p"`
	BuildNumber  string `help:"Deprecated; ignored because job UUIDs no longer require pipeline or build context" short:"b"`
	NoTimestamps bool   `help:"Strip timestamp prefixes from log output" name:"no-timestamps"`
	LLMOptimized bool   `help:"Format output to be optimal for LLM consumption (strips ANSI, deduplicates loops)" name:"agent"`
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
		logContent = formatForLLM(logContent)
	}

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	fmt.Fprint(writer, logContent)
	return nil
}

var timestampRegex = regexp.MustCompile(`bk;t=\d+\x07`)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// oscRegex matches OSC (`\x1b]...`) and APC (`\x1b_...`) escape sequences
// terminated by BEL (\x07) or ST (\x1b\\). Buildkite uses APC sequences for its
// inline timestamp metadata (e.g. `\x1b_bk;t=1700000000000\x07`).
var oscRegex = regexp.MustCompile("\x1b[\\]_][^\x07]*(?:\x07|\x1b\\\\)")

func stripTimestamps(content string) string {
	return timestampRegex.ReplaceAllString(content, "")
}

func formatForLLM(content string) string {
	content = ansiRegex.ReplaceAllString(content, "")
	content = oscRegex.ReplaceAllString(content, "")
	// Strip any bare timestamp markers that weren't wrapped in an APC sequence,
	// so deduplication works regardless of the --no-timestamps flag.
	content = stripTimestamps(content)

	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	var prevLine string
	hasPrev := false
	repeatCount := 0

	flush := func() {
		if repeatCount > 0 {
			result = append(result, fmt.Sprintf("[Previous line repeated %d times]", repeatCount))
			repeatCount = 0
		}
	}

	for _, line := range lines {
		// Collapse carriage-return redraws (progress bars, spinners): keep only
		// the final segment that would actually be visible in a terminal.
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}

		// Deduplicate consecutive identical lines, but never collapse blank lines.
		if line != "" && hasPrev && line == prevLine {
			repeatCount++
			continue
		}

		flush()

		result = append(result, transformHeader(line))
		prevLine = line
		hasPrev = true
	}

	flush()

	return strings.Join(result, "\n")
}

// transformHeader rewrites Buildkite log group markers (`---`, `+++`, `~~~`)
// into clear phase boundaries for an LLM. The marker must be a standalone token
// or followed by a space so that separators like `----------` are left intact.
func transformHeader(line string) string {
	for _, prefix := range []string{"---", "+++", "~~~"} {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			return "\n=== PHASE: " + strings.TrimPrefix(line, prefix) + " ==="
		}
	}
	return line
}
