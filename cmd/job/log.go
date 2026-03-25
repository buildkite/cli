package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	buildkitelogs "github.com/buildkite/buildkite-logs"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/logs"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
)

type LogCmd struct {
	// Positional arguments
	JobID string `arg:"" optional:"" help:"Job UUID or Buildkite URL (interactive picker if omitted)"`

	// Pipeline/build/job resolution
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}" short:"p"`
	BuildNumber string `help:"The build number" short:"b"`
	Step        string `help:"Step key from pipeline.yml to get logs for" short:"s"`

	// Reading flags
	Seek   int    `help:"Start reading from row N (0-based)" default:"-1"`
	Limit  int    `help:"Maximum number of lines to output" default:"0"`
	Tail   int    `help:"Show last N lines" short:"n" default:"0"`
	Follow bool   `help:"Follow log output for running jobs (poll every 2s)" short:"f"`
	Since  string `help:"Show logs after this time (e.g. 5m, 2h, or RFC3339 timestamp)" short:"S"`
	Until  string `help:"Show logs before this time (e.g. 5m, 2h, or RFC3339 timestamp)" short:"U"`

	// Filter flags
	Group string `help:"Filter logs to entries in a specific group/section" short:"G"`

	// Display flags
	Timestamps   bool `help:"Prefix each line with a human-readable timestamp" short:"t"`
	NoTimestamps bool `help:"Strip timestamp prefixes from log output" name:"no-timestamps"`

	// Output format
	JSON bool `help:"Output as JSON (one object per line)" name:"json"`

	// Cached parsed time values (set once in Run, used per-row in entryInTimeRange)
	sinceTime time.Time `kong:"-"`
	untilTime time.Time `kong:"-"`
}

func (c *LogCmd) Help() string {
	return `
Examples:
  # Get a job's full log
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 -p my-pipeline -b 123

  # Get logs from a Buildkite URL (copy-paste from web UI or Slack)
  $ bk logs https://buildkite.com/my-org/my-pipeline/builds/123#0190046e-e199-453b-a302-a21a4d649d31

  # Build URL without job fragment (opens job picker)
  $ bk logs https://buildkite.com/my-org/my-pipeline/builds/123

  # Get logs by step key (from pipeline.yml)
  $ bk job log -p my-pipeline -b 123 --step "test-suite"

  # Interactive job picker (omit job ID)
  $ bk job log -p my-pipeline -b 123

  # Show last 50 lines
  $ bk job log JOB_ID -b 123 -n 50

  # Follow a running job's log output
  $ bk job log JOB_ID -b 123 -f

  # Follow and search for errors (pipe to grep)
  $ bk job log JOB_ID -b 123 -f | grep -i "error\|panic"

  # Search with context (pipe to grep)
  $ bk job log JOB_ID -b 123 | grep -C 3 "error\|failed"

  # Show logs from the last 10 minutes
  $ bk job log JOB_ID -b 123 --since 10m

  # Show logs between two timestamps
  $ bk job log JOB_ID -b 123 --since 2024-01-15T10:00:00Z --until 2024-01-15T10:05:00Z

  # Show human-readable timestamps
  $ bk job log JOB_ID -b 123 -t

  # Filter to a specific group/section
  $ bk job log JOB_ID -b 123 -G "Running tests"

  # Output as JSON lines (for piping to jq)
  $ bk job log JOB_ID -b 123 --json | jq '.content'

  # Paginated read (rows 100-200)
  $ bk job log JOB_ID -b 123 --seek 100 --limit 100

  # Add line numbers (pipe to nl or cat -n)
  $ bk job log JOB_ID -b 123 | cat -n
`
}

func (c *LogCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	// If the positional arg is a Buildkite URL, extract org/pipeline/build/job from it.
	if parsed := parseJobURL(c.JobID); parsed != nil {
		if c.Pipeline != "" || c.BuildNumber != "" {
			return bkErrors.NewValidationError(
				fmt.Errorf("cannot use --pipeline or --build with a Buildkite URL"),
				"the URL already contains the pipeline and build number",
			)
		}
		c.Pipeline = parsed.org + "/" + parsed.pipeline
		c.BuildNumber = parsed.buildNumber
		c.JobID = parsed.jobID
	}

	if err := c.validateFlags(); err != nil {
		return err
	}

	// Cache parsed time values once so entryInTimeRange doesn't re-parse per row.
	// For duration-based values (e.g. "5m"), this pins time.Now() to invocation time,
	// ensuring deterministic filtering across the entire log.
	if c.Since != "" {
		c.sinceTime, _ = parseTimeFlag(c.Since)
	}
	if c.Until != "" {
		c.untilTime, _ = parseTimeFlag(c.Until)
	}

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

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)

	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	args := []string{}
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}
	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)

	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}
	if bld == nil {
		return bkErrors.NewResourceNotFoundError(nil, "no build found",
			"Check the build number and pipeline are correct",
			"Run 'bk build list' to see recent builds",
		)
	}

	// Resolve job: by step key, by positional job ID, or interactive picker
	var jobLabel string
	switch {
	case c.Step != "":
		picked, err := c.resolveJobByStepKey(ctx, f, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber))
		if err != nil {
			return err
		}
		c.JobID = picked.id
		jobLabel = picked.label
	case c.JobID == "":
		picked, err := c.pickJob(ctx, f, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber))
		if err != nil {
			return err
		}
		c.JobID = picked.id
		jobLabel = picked.label
	}

	// Create buildkite-logs client
	logsClient, err := logs.NewClient(ctx, f.RestAPIClient)
	if err != nil {
		return bkErrors.WrapAPIError(err, "creating logs client")
	}
	defer logsClient.Close()

	org := bld.Organization
	pipeline := bld.Pipeline
	build := fmt.Sprint(bld.BuildNumber)

	// Auto-follow when no explicit mode was requested and the job is still running.
	if c.shouldAutoFollow() && bkIO.IsTTY() {
		state, err := c.jobState(ctx, f, org, pipeline, build, c.JobID)
		if err == nil && !buildkitelogs.IsTerminalState(buildkitelogs.JobState(state)) {
			if jobLabel != "" {
				fmt.Fprintf(os.Stderr, "Job '%s' is still running — following log output (Ctrl-C to stop)...\n", jobLabel)
			} else {
				fmt.Fprintln(os.Stderr, "Job is still running — following log output (Ctrl-C to stop)...")
			}
			c.Follow = true
		}
	}

	// Dispatch to the appropriate mode.
	// Only unbounded full-log read uses the pager; tail and follow write directly to stdout.
	switch {
	case c.Follow:
		return c.followMode(ctx, f, logsClient, org, pipeline, build, c.JobID)
	case c.Tail > 0:
		return c.tailMode(ctx, f, logsClient, org, pipeline, build, c.JobID)
	default:
		return c.readMode(ctx, f, logsClient, org, pipeline, build, c.JobID)
	}
}

func (c *LogCmd) validateFlags() error {
	if c.Step != "" && c.JobID != "" {
		return bkErrors.NewValidationError(
			fmt.Errorf("--step and a positional job ID are mutually exclusive"),
			"use either --step or a job ID, not both",
		)
	}
	if c.Tail > 0 && c.Seek >= 0 {
		return bkErrors.NewValidationError(
			fmt.Errorf("--tail and --seek are mutually exclusive"),
			"use --tail to see the last N lines, or --seek to start from a specific row",
		)
	}
	if c.Follow && c.Seek >= 0 {
		return bkErrors.NewValidationError(
			fmt.Errorf("--follow and --seek cannot be used together"),
			"use --follow to stream new output, or --seek to read from a specific offset",
		)
	}
	if c.Timestamps && c.NoTimestamps {
		return bkErrors.NewValidationError(
			fmt.Errorf("--timestamps and --no-timestamps are mutually exclusive"),
			"use one or the other",
		)
	}
	if (c.Since != "" || c.Until != "") && c.Seek >= 0 {
		return bkErrors.NewValidationError(
			fmt.Errorf("--since/--until and --seek are mutually exclusive"),
			"use time-based filtering or row-based seeking, not both",
		)
	}
	if c.Follow && c.Until != "" {
		return bkErrors.NewValidationError(
			fmt.Errorf("--follow and --until cannot be used together"),
			"--follow streams indefinitely; --until sets an end time",
		)
	}
	if c.Since != "" {
		if _, err := parseTimeFlag(c.Since); err != nil {
			return bkErrors.NewValidationError(
				fmt.Errorf("invalid --since value: %w", err),
				"expected a duration (e.g. 5m, 2h) or RFC3339 timestamp",
			)
		}
	}
	if c.Until != "" {
		if _, err := parseTimeFlag(c.Until); err != nil {
			return bkErrors.NewValidationError(
				fmt.Errorf("invalid --until value: %w", err),
				"expected a duration (e.g. 5m, 2h) or RFC3339 timestamp",
			)
		}
	}
	return nil
}

// shouldAutoFollow returns true when no explicit mode flags were set,
// meaning the command should check job state and auto-follow if running.
func (c *LogCmd) shouldAutoFollow() bool {
	return !c.Follow && c.Tail <= 0 && c.Seek < 0 && c.Limit <= 0 && c.Since == "" && c.Until == ""
}

// parseTimeFlag parses a time value that is either a Go duration string (relative to now)
// or an RFC3339 timestamp (absolute).
func parseTimeFlag(value string) (time.Time, error) {
	// Try as a duration first (e.g. "5m", "2h", "30s")
	if d, err := time.ParseDuration(value); err == nil {
		return time.Now().Add(-d), nil
	}
	// Try as RFC3339 timestamp
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("must be a duration (e.g. 5m, 2h) or RFC3339 timestamp (e.g. 2024-01-15T10:00:00Z)")
}

// entryInTimeRange checks whether an entry's timestamp falls within the --since/--until range.
// Uses sinceTime/untilTime cached in Run() to ensure deterministic filtering.
func (c *LogCmd) entryInTimeRange(entry *buildkitelogs.ParquetLogEntry) bool {
	if c.Since == "" && c.Until == "" {
		return true
	}
	ts := entry.Timestamp // unix millis
	if c.Since != "" && ts < c.sinceTime.UnixMilli() {
		return false
	}
	if c.Until != "" && ts > c.untilTime.UnixMilli() {
		return false
	}
	return true
}

type cmdJob struct {
	id    string
	label string
	state string
}

// buildJobLabels creates display labels for job picker options.
// Duplicate labels are disambiguated with a short job ID suffix.
func buildJobLabels(jobs []cmdJob) []string {
	labels := make([]string, len(jobs))
	for i, j := range jobs {
		labels[i] = fmt.Sprintf("%s (%s)", j.label, j.state)
	}
	seen := make(map[string]int)
	for _, l := range labels {
		seen[l]++
	}
	for i, l := range labels {
		if seen[l] > 1 {
			shortID := jobs[i].id
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			labels[i] = fmt.Sprintf("%s [%s]", l, shortID)
		}
	}
	return labels
}

func (c *LogCmd) pickJob(ctx context.Context, f *factory.Factory, org, pipeline, buildNumber string) (cmdJob, error) {
	buildInfo, _, err := f.RestAPIClient.Builds.Get(ctx, org, pipeline, buildNumber, nil)
	if err != nil {
		return cmdJob{}, bkErrors.WrapAPIError(err, "fetching build to list jobs")
	}

	// Filter to command jobs only
	var commandJobs []cmdJob
	for _, j := range buildInfo.Jobs {
		if j.Type != "script" {
			continue
		}
		label := j.Label
		if label == "" {
			label = j.Name
		}
		if label == "" {
			label = j.Command
		}
		if len(label) > 60 {
			label = label[:57] + "..."
		}
		commandJobs = append(commandJobs, cmdJob{id: j.ID, label: label, state: j.State})
	}

	if len(commandJobs) == 0 {
		return cmdJob{}, bkErrors.NewResourceNotFoundError(nil,
			fmt.Sprintf("no command jobs found in build %s", buildNumber),
			"The build may only contain non-command steps (wait, block, trigger)",
		)
	}

	// Auto-select if only one job
	if len(commandJobs) == 1 {
		return commandJobs[0], nil
	}

	labels := buildJobLabels(commandJobs)

	chosen, err := bkIO.PromptForOne("job", labels, f.NoInput)
	if err != nil {
		return cmdJob{}, err
	}

	// Find the matching job by label
	for i, label := range labels {
		if label == chosen {
			return commandJobs[i], nil
		}
	}

	return cmdJob{}, fmt.Errorf("could not match job selection")
}

func (c *LogCmd) resolveJobByStepKey(ctx context.Context, f *factory.Factory, org, pipeline, buildNumber string) (cmdJob, error) {
	buildInfo, _, err := f.RestAPIClient.Builds.Get(ctx, org, pipeline, buildNumber, nil)
	if err != nil {
		return cmdJob{}, bkErrors.WrapAPIError(err, "fetching build to resolve step key")
	}

	var matches []cmdJob
	for _, j := range buildInfo.Jobs {
		if j.StepKey != c.Step {
			continue
		}
		label := j.Label
		if label == "" {
			label = j.Name
		}
		if label == "" {
			label = j.Command
		}
		// Append parallel index to label when present (e.g. "rspec #3")
		if j.ParallelGroupIndex != nil {
			label = fmt.Sprintf("%s #%d", label, *j.ParallelGroupIndex)
		}
		if len(label) > 60 {
			label = label[:57] + "..."
		}
		matches = append(matches, cmdJob{id: j.ID, label: label, state: j.State})
	}

	if len(matches) == 0 {
		return cmdJob{}, bkErrors.NewResourceNotFoundError(nil,
			fmt.Sprintf("no job found with step key %q in build %s", c.Step, buildNumber),
			"Check the step key matches your pipeline.yml",
			"Run 'bk job list' to see available jobs in this build",
		)
	}

	// Auto-select if only one match
	if len(matches) == 1 {
		return matches[0], nil
	}

	// Multiple matches (parallel matrix) — use interactive picker
	labels := buildJobLabels(matches)
	chosen, err := bkIO.PromptForOne("job", labels, f.NoInput)
	if err != nil {
		return cmdJob{}, err
	}
	for i, label := range labels {
		if label == chosen {
			return matches[i], nil
		}
	}

	return cmdJob{}, fmt.Errorf("could not match job selection")
}

func (c *LogCmd) readMode(ctx context.Context, f *factory.Factory, logsClient *buildkitelogs.Client, org, pipeline, build, jobID string) error {
	var reader *buildkitelogs.ParquetReader
	var readerErr error
	_ = bkIO.SpinWhile(f, "Fetching job log", func() {
		reader, readerErr = logsClient.NewReader(ctx, org, pipeline, build, jobID, 30*time.Second, false)
	})
	err := readerErr
	if err != nil {
		return c.handleLogError(err)
	}
	defer reader.Close()

	var entryIter func(func(buildkitelogs.ParquetLogEntry, error) bool)
	switch {
	case c.Seek >= 0:
		entryIter = reader.SeekToRow(int64(c.Seek))
	case c.Group != "":
		entryIter = reader.FilterByGroupIter(c.Group)
	default:
		entryIter = reader.ReadEntriesIter()
	}

	// Use pager only for unbounded full-log reads (not JSON output)
	usePager := c.Limit <= 0 && c.Seek < 0 && !c.isJSONOutput()
	var writer io.Writer = os.Stdout
	var cleanup func() error
	if usePager {
		writer, cleanup = bkIO.Pager(f.NoPager, f.Config.Pager())
		defer func() { _ = cleanup() }()
	}

	count := 0
	for entry, iterErr := range entryIter {
		if iterErr != nil {
			return fmt.Errorf("failed to read log entries: %w", iterErr)
		}
		if !c.entryInTimeRange(&entry) {
			continue
		}
		c.writeEntry(writer, &entry)
		count++
		if c.Limit > 0 && count >= c.Limit {
			break
		}
	}

	if count == 0 {
		fmt.Fprintln(os.Stderr, "No log output for this job.")
	}

	return nil
}

func (c *LogCmd) tailMode(ctx context.Context, f *factory.Factory, logsClient *buildkitelogs.Client, org, pipeline, build, jobID string) error {
	var reader *buildkitelogs.ParquetReader
	var readerErr error
	_ = bkIO.SpinWhile(f, "Fetching job log", func() {
		reader, readerErr = logsClient.NewReader(ctx, org, pipeline, build, jobID, 30*time.Second, false)
	})
	err := readerErr
	if err != nil {
		return c.handleLogError(err)
	}
	defer reader.Close()

	fileInfo, err := reader.GetFileInfo()
	if err != nil {
		return fmt.Errorf("failed to get log info: %w", err)
	}

	if fileInfo.RowCount == 0 {
		fmt.Fprintln(os.Stderr, "No log output for this job.")
		return nil
	}

	// When time filtering is active, we need to scan all entries and take the last N that match.
	// Without time filtering, we can efficiently seek to the right offset.
	if c.Since != "" || c.Until != "" {
		var matched []buildkitelogs.ParquetLogEntry
		iter := reader.ReadEntriesIter()
		if c.Group != "" {
			iter = reader.FilterByGroupIter(c.Group)
		}
		for entry, iterErr := range iter {
			if iterErr != nil {
				return fmt.Errorf("failed to read tail entries: %w", iterErr)
			}
			if c.entryInTimeRange(&entry) {
				matched = append(matched, entry)
			}
		}
		start := max(len(matched)-c.Tail, 0)
		for _, entry := range matched[start:] {
			c.writeEntry(os.Stdout, &entry)
		}
		if len(matched) == 0 {
			fmt.Fprintln(os.Stderr, "No log output matching time range.")
		}
		return nil
	}

	startRow := max(fileInfo.RowCount-int64(c.Tail), 0)

	for entry, iterErr := range reader.SeekToRow(startRow) {
		if iterErr != nil {
			return fmt.Errorf("failed to read tail entries: %w", iterErr)
		}
		c.writeEntry(os.Stdout, &entry)
	}

	return nil
}

func (c *LogCmd) followMode(ctx context.Context, f *factory.Factory, logsClient *buildkitelogs.Client, org, pipeline, build, jobID string) error {
	// If --tail is set with --follow, show last N lines first then follow
	lastSeenRow := int64(0)

	// Initial fetch to get current state
	reader, err := logsClient.NewReader(ctx, org, pipeline, build, jobID, 30*time.Second, false)
	if err != nil {
		return c.handleLogError(err)
	}

	fileInfo, err := reader.GetFileInfo()
	if err != nil {
		reader.Close()
		return fmt.Errorf("failed to get log info: %w", err)
	}

	// Show initial content if --tail is set
	if c.Tail > 0 && fileInfo.RowCount > 0 {
		startRow := max(fileInfo.RowCount-int64(c.Tail), 0)
		for entry, iterErr := range reader.SeekToRow(startRow) {
			if iterErr != nil {
				reader.Close()
				return fmt.Errorf("failed to read initial entries: %w", iterErr)
			}
			if c.entryInTimeRange(&entry) {
				c.writeEntry(os.Stdout, &entry)
			}
		}
		lastSeenRow = fileInfo.RowCount
	} else {
		// Show everything from the beginning (respecting --since if set)
		for entry, iterErr := range reader.ReadEntriesIter() {
			if iterErr != nil {
				reader.Close()
				return fmt.Errorf("failed to read entries: %w", iterErr)
			}
			if c.entryInTimeRange(&entry) {
				c.writeEntry(os.Stdout, &entry)
			}
		}
		lastSeenRow = fileInfo.RowCount
	}
	reader.Close()

	// Check if job is already finished
	if c.isJobTerminal(ctx, f, org, pipeline, build, jobID) {
		return nil
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	consecutiveErrors := 0
	const maxConsecutiveErrors = 10

	for {
		select {
		case <-sigCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			reader, err := logsClient.NewReader(ctx, org, pipeline, build, jobID, 0, true)
			if err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					return bkErrors.WrapAPIError(err, fmt.Sprintf("fetching logs (%d consecutive failures)", consecutiveErrors))
				}
				continue
			}
			consecutiveErrors = 0

			fileInfo, err := reader.GetFileInfo()
			if err != nil {
				reader.Close()
				continue
			}

			if fileInfo.RowCount > lastSeenRow {
				processed := int64(0)
				for entry, iterErr := range reader.SeekToRow(lastSeenRow) {
					if iterErr != nil {
						break
					}
					c.writeEntry(os.Stdout, &entry)
					processed++
				}
				lastSeenRow += processed
			}
			reader.Close()

			if c.isJobTerminal(ctx, f, org, pipeline, build, jobID) {
				return nil
			}
		}
	}
}

// jobState returns the job's state string, or an error if the job/build can't be found.
func (c *LogCmd) jobState(ctx context.Context, f *factory.Factory, org, pipeline, build, jobID string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	buildInfo, _, err := f.RestAPIClient.Builds.Get(reqCtx, org, pipeline, build, nil)
	if err != nil {
		return "", err
	}
	for _, j := range buildInfo.Jobs {
		if j.ID == jobID {
			return j.State, nil
		}
	}
	return "", fmt.Errorf("job %s not found in build %s", jobID, build)
}

func (c *LogCmd) isJobTerminal(ctx context.Context, f *factory.Factory, org, pipeline, build, jobID string) bool {
	state, err := c.jobState(ctx, f, org, pipeline, build, jobID)
	if err != nil {
		return false
	}
	return buildkitelogs.IsTerminalState(buildkitelogs.JobState(state))
}

func (c *LogCmd) writeEntry(w io.Writer, entry *buildkitelogs.ParquetLogEntry) {
	if c.isJSONOutput() {
		c.writeEntryJSON(w, entry)
		return
	}

	content := entry.CleanContent(!output.ColorEnabled())

	// --timestamps: replace raw bk;t= markers with human-readable prefix
	if c.Timestamps {
		content = stripTimestamps(content)
		ts := time.UnixMilli(entry.Timestamp).UTC().Format(time.RFC3339)
		content = ts + " " + content
	} else if c.NoTimestamps {
		content = stripTimestamps(content)
	}

	content = strings.TrimRight(content, "\n")
	fmt.Fprintf(w, "%s\n", content)
}

// logEntryJSON is the JSON representation of a log entry.
type logEntryJSON struct {
	RowNumber int64  `json:"row_number"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
	Group     string `json:"group,omitempty"`
}

func (c *LogCmd) writeEntryJSON(w io.Writer, entry *buildkitelogs.ParquetLogEntry) {
	obj := logEntryJSON{
		RowNumber: entry.RowNumber,
		Timestamp: time.UnixMilli(entry.Timestamp).UTC().Format(time.RFC3339),
		Content:   strings.TrimRight(entry.CleanContent(true), "\n"),
		Group:     entry.Group,
	}
	data, _ := json.Marshal(obj)
	fmt.Fprintf(w, "%s\n", data)
}

// isJSONOutput returns true if JSON output format is selected.
func (c *LogCmd) isJSONOutput() bool {
	return c.JSON
}

func (c *LogCmd) handleLogError(err error) error {
	if errors.Is(err, buildkitelogs.ErrLogTooLarge) {
		return bkErrors.NewValidationError(err, "log exceeds maximum size",
			"Use --tail N to see the last N lines",
			"Use --seek/--limit to read a specific portion",
		)
	}
	return bkErrors.WrapAPIError(err, "fetching job log")
}

var timestampRegex = regexp.MustCompile(`bk;t=\d+\x07`)

func stripTimestamps(content string) string {
	return timestampRegex.ReplaceAllString(content, "")
}

// parsedJobURL holds the components extracted from a Buildkite job URL.
type parsedJobURL struct {
	org         string
	pipeline    string
	buildNumber string
	jobID       string
}

// buildkiteURLRegex matches Buildkite build URLs with an optional #job-uuid fragment:
//
//	https://buildkite.com/org/pipeline/builds/123
//	https://buildkite.com/org/pipeline/builds/123#job-uuid
var buildkiteURLRegex = regexp.MustCompile(`^https?://buildkite\.com/([^/]+)/([^/]+)/builds/(\d+)(?:#([0-9a-fA-F-]+))?$`)

// parseJobURL extracts org, pipeline, build number, and optionally job ID from a Buildkite URL.
// Returns nil if the input is not a recognized Buildkite build/job URL.
// Handles common copy-paste artifacts like Slack's angle-bracket wrapping (<url>).
func parseJobURL(input string) *parsedJobURL {
	input = strings.TrimSpace(input)
	// Strip Slack-style angle brackets: <https://...>
	input = strings.TrimPrefix(input, "<")
	input = strings.TrimSuffix(input, ">")
	m := buildkiteURLRegex.FindStringSubmatch(input)
	if m == nil {
		return nil
	}
	return &parsedJobURL{
		org:         m[1],
		pipeline:    m[2],
		buildNumber: m[3],
		jobID:       m[4], // empty string if no fragment
	}
}
