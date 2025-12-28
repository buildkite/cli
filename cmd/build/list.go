package build

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

const (
	maxBuildLimit = 5000
	pageSize      = 100
)

type ListCmd struct {
	Pipeline string            `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Since    string            `help:"Filter builds created since this time (e.g. 1h, 30m)"`
	Until    string            `help:"Filter builds created before this time (e.g. 1h, 30m)"`
	Duration string            `help:"Filter by duration (e.g. >5m, <10m, 20m) - supports >, <, >=, <= operators"`
	State    []string          `help:"Filter by build state"`
	Branch   []string          `help:"Filter by branch name"`
	Creator  string            `help:"Filter by creator (email address or user ID)"`
	Commit   string            `help:"Filter by commit SHA"`
	Message  string            `help:"Filter by message content"`
	MetaData map[string]string `help:"Filter by build meta-data (key=value format, can be specified multiple times)"`
	Limit    int               `help:"Maximum number of builds to return" default:"50"`
	NoLimit  bool              `help:"Fetch all builds (overrides --limit)"`
	Output   string            `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ListCmd) Help() string {
	return `List builds with optional filtering.

This command supports both server-side filtering (fast) and client-side filtering.
Server-side filters are applied by the Buildkite API, while client-side filters
are applied after fetching results and may require loading more builds.

Client-side filters: --duration, --message
Server-side filters: --pipeline, --since, --until, --state, --branch, --creator, --commit, --meta-data

Builds can be filtered by their duration, message content, and other attributes.
When filtering by duration, you can use operators like >, <, >=, and <= to specify your criteria.
Supported duration units are seconds (s), minutes (m), and hours (h).

Examples:
  # List recent builds (50 by default)
  $ bk build list

  # Get more builds (automatically paginates)
  $ bk build list --limit 500

  # List builds from the last hour
  $ bk build list --since 1h

  # List failed builds
  $ bk build list --state failed

  # List builds on main branch
  $ bk build list --branch main

  # List builds by alice
  $ bk build list --creator alice@company.com

  # List builds that took longer than 20 minutes
  $ bk build list --duration ">20m"

  # List builds that finished in under 5 minutes
  $ bk build list --duration "<5m"

  # Combine filters: failed builds on main branch in the last 24 hours
  $ bk build list --state failed --branch main --since 24h

  # Find builds containing "deploy" in the message
  $ bk build list --message deploy

  # Filter builds by meta-data
  $ bk build list --meta-data env=production

  # Filter by multiple meta-data keys
  $ bk build list --meta-data env=production --meta-data deploy=true

  # Complex filtering: slow builds (>30m) that failed on feature branches
  $ bk build list --duration ">30m" --state failed --branch feature/`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
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

	if !c.NoLimit {
		if c.Limit > maxBuildLimit {
			return fmt.Errorf("limit cannot exceed %d builds (requested: %d); if you need more, use --no-limit", maxBuildLimit, c.Limit)
		}
	}

	if c.Creator != "" && isValidEmail(c.Creator) {
		originalEmail := c.Creator
		err = bkIO.SpinWhile(f, "Looking up user", func() {
			c.Creator, err = resolveCreatorEmailToUserID(ctx, f, originalEmail)
		})
		if err != nil {
			return fmt.Errorf("failed to resolve creator email: %w", err)
		}
		if c.Creator == "" {
			return fmt.Errorf("failed to resolve creator email: no user found")
		}
	}

	listOpts, err := c.buildListOptions()
	if err != nil {
		return err
	}

	org := f.Config.OrganizationSlug()

	format := output.Format(c.Output)
	builds, err := c.fetchBuilds(ctx, f, org, listOpts)
	if err != nil {
		return fmt.Errorf("failed to list builds: %w", err)
	}

	if len(builds) == 0 {
		fmt.Println("No builds found matching the specified criteria.")
		return nil
	}

	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager)
		defer func() { _ = cleanup() }()
		return displayBuilds(builds, format, true, writer)
	}

	return displayBuilds(builds, format, false, os.Stdout)
}

func (c *ListCmd) buildListOptions() (*buildkite.BuildsListOptions, error) {
	listOpts := &buildkite.BuildsListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: pageSize,
		},
	}

	now := time.Now()
	if c.Since != "" {
		d, err := time.ParseDuration(c.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since duration '%s': %w", c.Since, err)
		}
		listOpts.CreatedFrom = now.Add(-d)
	}

	if c.Until != "" {
		d, err := time.ParseDuration(c.Until)
		if err != nil {
			return nil, fmt.Errorf("invalid until duration '%s': %w", c.Until, err)
		}
		listOpts.CreatedTo = now.Add(-d)
	}

	if len(c.State) > 0 {
		listOpts.State = make([]string, len(c.State))
		for i, state := range c.State {
			listOpts.State[i] = strings.ToLower(state)
		}
	}

	listOpts.Branch = c.Branch
	listOpts.Creator = c.Creator
	listOpts.Commit = c.Commit

	if len(c.MetaData) > 0 {
		listOpts.MetaData = buildkite.MetaDataFilters{
			MetaData: c.MetaData,
		}
	}

	return listOpts, nil
}

func (c *ListCmd) fetchBuilds(ctx context.Context, f *factory.Factory, org string, listOpts *buildkite.BuildsListOptions) ([]buildkite.Build, error) {
	var allBuilds []buildkite.Build

	// filtered builds added since last confirm (used when --no-limit)
	filteredSinceConfirm := 0

	// raw (unfiltered) build counters so progress messaging makes sense when client-side filters are active
	rawTotalFetched := 0
	rawSinceConfirm := 0
	previousPageFirstBuildNumber := 0

	format := output.Format(c.Output)

	for page := 1; ; page++ {
		if !c.NoLimit && len(allBuilds) >= c.Limit {
			break
		}

		listOpts.Page = page

		var builds []buildkite.Build
		var err error

		spinnerMsg := "Loading builds ("
		if c.Pipeline != "" {
			spinnerMsg += fmt.Sprintf("pipeline %s, ", c.Pipeline)
		}
		filtersActive := c.Duration != "" || c.Message != ""

		// Show matching (filtered) counts and raw counts independently
		if !c.NoLimit && c.Limit > 0 {
			spinnerMsg += fmt.Sprintf("%d/%d matching, %d raw fetched", len(allBuilds), c.Limit, rawTotalFetched)
		} else {
			spinnerMsg += fmt.Sprintf("%d matching, %d raw fetched", len(allBuilds), rawTotalFetched)
		}
		spinnerMsg += ")"

		if format == output.FormatText && rawSinceConfirm >= maxBuildLimit {
			prompt := fmt.Sprintf("Fetched %d more builds (%d total). Continue?", rawSinceConfirm, rawTotalFetched)
			if filtersActive {
				prompt = fmt.Sprintf(
					"Fetched %d raw builds (%d matching, %d matching total). Continue?",
					rawSinceConfirm, filteredSinceConfirm, len(allBuilds),
				)
			}

			confirmed, err := bkIO.Confirm(f, prompt)
			if err != nil {
				return nil, err
			}

			if !confirmed {
				return allBuilds, nil
			}

			filteredSinceConfirm = 0
			rawSinceConfirm = 0
		}

		spinErr := bkIO.SpinWhile(f, spinnerMsg, func() {
			if c.Pipeline != "" {
				builds, err = c.getBuildsByPipeline(ctx, f, org, listOpts)
			} else {
				builds, _, err = f.RestAPIClient.Builds.ListByOrg(ctx, org, listOpts)
			}
		})

		if spinErr != nil {
			return nil, spinErr
		}

		if err != nil {
			return nil, err
		}

		if len(builds) == 0 {
			break
		}

		// Track raw builds fetched before applying client-side filters
		rawCountThisPage := len(builds)
		rawTotalFetched += rawCountThisPage
		rawSinceConfirm += rawCountThisPage

		// Detect duplicate first build number between pages to prevent infinite loop
		if page > 1 && len(builds) > 0 {
			currentPageFirstBuildNumber := builds[0].Number
			if currentPageFirstBuildNumber == previousPageFirstBuildNumber {
				return nil, fmt.Errorf("API returned duplicate results, stopping to prevent infinite loop")
			}
		}

		if len(builds) > 0 {
			previousPageFirstBuildNumber = builds[0].Number
		}

		builds, err = c.applyClientSideFilters(builds)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filters: %w", err)
		}

		// Decide which builds will actually be added (respect limit)
		var buildsToAdd []buildkite.Build
		addedThisPage := 0
		if !c.NoLimit {
			remaining := c.Limit - len(allBuilds)
			if remaining <= 0 {
				break
			}
			if len(builds) > remaining {
				buildsToAdd = builds[:remaining]
				addedThisPage = remaining
			} else {
				buildsToAdd = builds
				addedThisPage = len(builds)
			}
		} else {
			buildsToAdd = builds
			addedThisPage = len(builds)
		}

		allBuilds = append(allBuilds, buildsToAdd...)
		filteredSinceConfirm += addedThisPage

		if rawCountThisPage < listOpts.PerPage {
			break
		}
	}

	return allBuilds, nil
}

func (c *ListCmd) getBuildsByPipeline(ctx context.Context, f *factory.Factory, org string, listOpts *buildkite.BuildsListOptions) ([]buildkite.Build, error) {
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
	)

	pipeline, err := pipelineRes.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, org, pipeline.Name, listOpts)
	return builds, err
}

func (c *ListCmd) applyClientSideFilters(builds []buildkite.Build) ([]buildkite.Build, error) {
	if c.Duration == "" && c.Message == "" {
		return builds, nil
	}

	var durationOp string
	var durationThreshold time.Duration

	if c.Duration != "" {
		durationOp = ">="
		durationStr := c.Duration

		switch {
		case strings.HasPrefix(c.Duration, "<"):
			durationOp = "<"
			durationStr = c.Duration[1:]
		case strings.HasPrefix(c.Duration, ">"):
			durationOp = ">"
			durationStr = c.Duration[1:]
		}

		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format: %w", err)
		}
		durationThreshold = d
	}

	var messageFilter string
	if c.Message != "" {
		messageFilter = strings.ToLower(c.Message)
	}

	var result []buildkite.Build
	for _, build := range builds {
		if c.Duration != "" {
			if build.StartedAt == nil {
				continue
			}

			var elapsed time.Duration
			if build.FinishedAt != nil {
				elapsed = build.FinishedAt.Sub(build.StartedAt.Time)
			} else {
				elapsed = time.Since(build.StartedAt.Time)
			}

			switch durationOp {
			case "<":
				if elapsed >= durationThreshold {
					continue
				}
			case ">":
				if elapsed <= durationThreshold {
					continue
				}
			default:
				if elapsed < durationThreshold {
					continue
				}
			}
		}

		if messageFilter != "" {
			if !strings.Contains(strings.ToLower(build.Message), messageFilter) {
				continue
			}
		}

		result = append(result, build)
	}

	return result, nil
}

func isValidEmail(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil
}

func resolveCreatorEmailToUserID(ctx context.Context, f *factory.Factory, email string) (string, error) {
	org := f.Config.OrganizationSlug()
	resp, err := graphql.FindUserByEmail(ctx, f.GraphQLClient, org, email)
	if err != nil {
		return "", fmt.Errorf("failed to query user by email: %w", err)
	}

	if resp.Organization == nil || resp.Organization.Members == nil || len(resp.Organization.Members.Edges) == 0 {
		return "", fmt.Errorf("no user found with email: %s", email)
	}

	member := resp.Organization.Members.Edges[0].Node
	if member == nil {
		return "", fmt.Errorf("invalid user data for email: %s", email)
	}

	// Decode GraphQL ID and extract UUID
	decoded, err := base64.StdEncoding.DecodeString(member.User.Id)
	if err != nil {
		return "", fmt.Errorf("failed to decode user ID: %w", err)
	}

	if userUUID, found := strings.CutPrefix(string(decoded), "User---"); found {
		return userUUID, nil
	}

	return "", fmt.Errorf("unexpected user ID format")
}

func displayBuilds(builds []buildkite.Build, format output.Format, _ bool, writer io.Writer) error {
	if format != output.FormatText {
		return output.Write(writer, builds, format)
	}

	const (
		maxMessageLength = 22
		truncatedLength  = 19
		timeFormat       = "2006-01-02T15:04:05Z"
	)

	headers := []string{"Number", "State", "Message", "Started (UTC)", "Finished (UTC)", "Duration", "URL"}
	var rows [][]string

	for _, build := range builds {
		message := build.Message
		if len(message) > maxMessageLength {
			message = message[:truncatedLength] + "..."
		}

		startedAt := "-"
		if build.StartedAt != nil {
			startedAt = build.StartedAt.Format(timeFormat)
		}

		finishedAt := "-"
		duration := "-"
		if build.FinishedAt != nil {
			finishedAt = build.FinishedAt.Format(timeFormat)
			if build.StartedAt != nil {
				dur := build.FinishedAt.Sub(build.StartedAt.Time)
				duration = formatDuration(dur)
			}
		} else if build.StartedAt != nil {
			dur := time.Since(build.StartedAt.Time)
			duration = formatDuration(dur) + " (running)"
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", build.Number),
			build.State,
			message,
			startedAt,
			finishedAt,
			duration,
			build.WebURL,
		})
	}

	table := output.Table(headers, rows, map[string]string{
		"number":         "bold",
		"state":          "bold",
		"message":        "italic",
		"started (utc)":  "dim",
		"finished (utc)": "dim",
		"duration":       "bold",
		"url":            "dim",
	})

	fmt.Fprint(writer, table)
	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := d / time.Minute
		seconds := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
