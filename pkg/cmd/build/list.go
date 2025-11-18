package build

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

const (
	maxBuildLimit = 5000
	pageSize      = 100
)

var (
	DisplayBuildsFunc = displayBuilds
	ConfirmFunc       = io.Confirm
)

type buildListOptions struct {
	pipeline string
	since    string
	until    string
	duration string
	state    []string
	branch   []string
	creator  string
	commit   string
	message  string
	limit    int
	noLimit  bool
}

func NewCmdBuildList(f *factory.Factory) *cobra.Command {
	var opts buildListOptions

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [flags]",
		Short:                 "List builds",
		Long: heredoc.Doc(`
			List builds with optional filtering.

			This command supports both server-side filtering (fast) and client-side filtering.
			Server-side filters are applied by the Buildkite API, while client-side filters
			are applied after fetching results and may require loading more builds.

			Client-side filters: --duration, --message
			Server-side filters: --pipeline, --since, --until, --state, --branch, --creator, --commit

			Builds can be filtered by their duration, message content, and other attributes.
			When filtering by duration, you can use operators like >, <, >=, and <= to specify your criteria.
			Supported duration units are seconds (s), minutes (m), and hours (h).
		`),
		Example: heredoc.Doc(`
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

			# Complex filtering: slow builds (>30m) that failed on feature branches
			$ bk build list --duration ">30m" --state failed --branch feature/
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			cmdScopes := scopes.GetCommandScopes(cmd)
			tokenScopes := f.Config.GetTokenScopes()
			if len(tokenScopes) == 0 {
				return fmt.Errorf("no scopes found in token. Please ensure you're using a token with appropriate scopes")
			}

			if err := scopes.ValidateScopes(cmdScopes, tokenScopes); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			// Get pipeline from persistent flag
			opts.pipeline, _ = cmd.Flags().GetString("pipeline")

			if !opts.noLimit {
				if opts.limit > maxBuildLimit {
					return fmt.Errorf("limit cannot exceed %d builds (requested: %d); if you need more, use --no-limit", maxBuildLimit, opts.limit)
				}
			}

			if opts.creator != "" && isValidEmail(opts.creator) {
				originalEmail := opts.creator
				err = io.SpinWhile("Looking up user", func() {
					opts.creator, err = resolveCreatorEmailToUserID(cmd.Context(), f, originalEmail)
				})
				if err != nil {
					return fmt.Errorf("failed to resolve creator email: %w", err)
				}
				if opts.creator == "" {
					return fmt.Errorf("failed to resolve creator email: no user found")
				}
			}

			listOpts, err := buildListOptionsFromFlags(&opts)
			if err != nil {
				return err
			}

			org := f.Config.OrganizationSlug()
			builds, err := fetchBuilds(cmd, f, org, opts, listOpts, format)
			if err != nil {
				return fmt.Errorf("failed to list builds: %w", err)
			}

			if len(builds) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No builds found matching the specified criteria.")
				return nil
			}

			if format == output.FormatText {
				return nil
			}

			return DisplayBuildsFunc(cmd, builds, format, false)
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.ReadBuilds),
	}

	// Pipeline flag now inherited from parent command
	cmd.Flags().StringVar(&opts.since, "since", "", "Filter builds created since this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.until, "until", "", "Filter builds created before this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.duration, "duration", "", "Filter by duration (e.g. >5m, <10m, 20m) - supports >, <, >=, <= operators")
	cmd.Flags().StringSliceVar(&opts.state, "state", []string{}, "Filter by build state")
	cmd.Flags().StringSliceVar(&opts.branch, "branch", []string{}, "Filter by branch name")
	cmd.Flags().StringVar(&opts.creator, "creator", "", "Filter by creator (email address or user ID)")
	cmd.Flags().StringVar(&opts.commit, "commit", "", "Filter by commit SHA")
	cmd.Flags().StringVar(&opts.message, "message", "", "Filter by message content")
	cmd.Flags().IntVar(&opts.limit, "limit", 50, fmt.Sprintf("Maximum number of builds to return (max: %d)", maxBuildLimit))
	cmd.Flags().BoolVar(&opts.noLimit, "no-limit", false, "Fetch all builds (overrides --limit)")

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false

	return &cmd
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

func buildListOptionsFromFlags(opts *buildListOptions) (*buildkite.BuildsListOptions, error) {
	listOpts := &buildkite.BuildsListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: pageSize,
		},
	}

	now := time.Now()
	if opts.since != "" {
		d, err := time.ParseDuration(opts.since)
		if err != nil {
			return nil, fmt.Errorf("invalid since duration '%s': %w", opts.since, err)
		}
		listOpts.CreatedFrom = now.Add(-d)
	}

	if opts.until != "" {
		d, err := time.ParseDuration(opts.until)
		if err != nil {
			return nil, fmt.Errorf("invalid until duration '%s': %w", opts.until, err)
		}
		listOpts.CreatedTo = now.Add(-d)
	}

	if len(opts.state) > 0 {
		listOpts.State = make([]string, len(opts.state))
		for i, state := range opts.state {
			listOpts.State[i] = strings.ToLower(state)
		}
	}

	listOpts.Branch = opts.branch
	listOpts.Creator = opts.creator
	listOpts.Commit = opts.commit

	return listOpts, nil
}

func fetchBuilds(cmd *cobra.Command, f *factory.Factory, org string, opts buildListOptions, listOpts *buildkite.BuildsListOptions, format output.Format) ([]buildkite.Build, error) {
	ctx := cmd.Context()
	var allBuilds []buildkite.Build

	// Track whether we've displayed any builds yet (for header logic)
	printedAny := false

	// filtered builds added since last confirm (used when --no-limit)
	filteredSinceConfirm := 0

	// raw (unfiltered) build counters so progress messaging makes sense when client-side filters are active
	rawTotalFetched := 0
	rawSinceConfirm := 0
	previousPageFirstBuildNumber := 0

	for page := 1; ; page++ {
		if !opts.noLimit && len(allBuilds) >= opts.limit {
			break
		}

		listOpts.Page = page

		var builds []buildkite.Build
		var err error

		spinnerMsg := "Loading builds ("
		if opts.pipeline != "" {
			spinnerMsg += fmt.Sprintf("pipeline %s, ", opts.pipeline)
		}
		filtersActive := opts.duration != "" || opts.message != ""

		// Show matching (filtered) counts and raw counts independently
		if !opts.noLimit && opts.limit > 0 {
			spinnerMsg += fmt.Sprintf("%d/%d matching, %d raw fetched", len(allBuilds), opts.limit, rawTotalFetched)
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

			confirmed, err := ConfirmFunc(f, prompt)
			if err != nil {
				return nil, err
			}

			if !confirmed {
				return allBuilds, nil
			}

			filteredSinceConfirm = 0
			rawSinceConfirm = 0
		}

		spinErr := io.SpinWhile(spinnerMsg, func() {
			if opts.pipeline != "" {
				builds, err = getBuildsByPipeline(ctx, f, org, opts.pipeline, listOpts)
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
				return nil, fmt.Errorf("API returned duplicate results, stopping to prevent infinite loop") // We should never get here
			}
		}

		if len(builds) > 0 {
			previousPageFirstBuildNumber = builds[0].Number
		}

		builds, err = applyClientSideFilters(builds, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filters: %w", err)
		}

		// Decide which builds will actually be added (respect limit)
		var buildsToAdd []buildkite.Build
		addedThisPage := 0
		if !opts.noLimit {
			remaining := opts.limit - len(allBuilds)
			if remaining <= 0 { // safety, though we check at loop top
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

		// Stream only the builds we are about to add; header only once we actually print something
		if format == output.FormatText && DisplayBuildsFunc != nil && len(buildsToAdd) > 0 {
			showHeader := !printedAny
			_ = DisplayBuildsFunc(cmd, buildsToAdd, format, showHeader)
			printedAny = true
		}

		allBuilds = append(allBuilds, buildsToAdd...)
		filteredSinceConfirm += addedThisPage

		if rawCountThisPage < listOpts.PerPage {
			break
		}
	}

	return allBuilds, nil
}

func getBuildsByPipeline(ctx context.Context, f *factory.Factory, org, pipelineFlag string, listOpts *buildkite.BuildsListOptions) ([]buildkite.Build, error) {
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(pipelineFlag, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
	)

	pipeline, err := pipelineRes.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, org, pipeline.Name, listOpts)
	return builds, err
}

func applyClientSideFilters(builds []buildkite.Build, opts buildListOptions) ([]buildkite.Build, error) {
	if opts.duration == "" && opts.message == "" {
		return builds, nil
	}

	var durationOp string
	var durationThreshold time.Duration

	if opts.duration != "" {
		durationOp = ">="
		durationStr := opts.duration

		switch {
		case strings.HasPrefix(opts.duration, "<"):
			durationOp = "<"
			durationStr = opts.duration[1:]
		case strings.HasPrefix(opts.duration, ">"):
			durationOp = ">"
			durationStr = opts.duration[1:]
		}

		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format: %w", err)
		}
		durationThreshold = d
	}

	var messageFilter string
	if opts.message != "" {
		messageFilter = strings.ToLower(opts.message)
	}

	var result []buildkite.Build
	for _, build := range builds {
		if opts.duration != "" {
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

func displayBuilds(cmd *cobra.Command, builds []buildkite.Build, format output.Format, withHeader bool) error {
	if format != output.FormatText {
		return output.Write(cmd.OutOrStdout(), builds, format)
	}

	const (
		maxMessageLength = 22
		truncatedLength  = 19
		timeFormat       = "2006-01-02T15:04:05Z"
		numberWidth      = 8
		stateWidth       = 12
		messageWidth     = 25
		timeWidth        = 20
		durationWidth    = 12
		columnSpacing    = 6
	)

	var buf strings.Builder

	if withHeader {
		header := lipgloss.NewStyle().Bold(true).Underline(true).Render("Builds")
		buf.WriteString(header)
		buf.WriteString("\n\n")

		headerRow := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s %s",
			numberWidth, "Number",
			stateWidth, "State",
			messageWidth, "Message",
			timeWidth, "Started (UTC)",
			timeWidth, "Finished (UTC)",
			durationWidth, "Duration",
			"URL")
		buf.WriteString(lipgloss.NewStyle().Bold(true).Render(headerRow))
		buf.WriteString("\n")
		totalWidth := numberWidth + stateWidth + messageWidth + timeWidth*2 + durationWidth + columnSpacing
		buf.WriteString(strings.Repeat("-", totalWidth))
		buf.WriteString("\n")
	}

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

		stateColor := getStateColor(build.State)
		coloredState := stateColor.Render(build.State)

		row := fmt.Sprintf("%-*d %-*s %-*s %-*s %-*s %-*s %s",
			numberWidth, build.Number,
			stateWidth, coloredState,
			messageWidth, message,
			timeWidth, startedAt,
			timeWidth, finishedAt,
			durationWidth, duration,
			build.WebURL)
		buf.WriteString(row)
		buf.WriteString("\n")
	}

	fmt.Fprint(cmd.OutOrStdout(), buf.String())
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

func getStateColor(state string) lipgloss.Style {
	switch state {
	case "passed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // Red
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // Yellow
	case "canceled", "cancelled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray
	case "scheduled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // Blue
	default:
		return lipgloss.NewStyle()
	}
}
