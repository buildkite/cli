package build

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
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
}

func NewCmdBuildList(f *factory.Factory) *cobra.Command {
	var opts buildListOptions

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [flags]",
		Short:                 "List builds",
		Long: heredoc.Doc(`
			List builds with optional filtering.
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

			if opts.limit > maxBuildLimit {
				return fmt.Errorf("limit cannot exceed %d builds (requested: %d)", maxBuildLimit, opts.limit)
			}

			listOpts, err := buildListOptionsFromFlags(&opts)
			if err != nil {
				return err
			}

			org := f.Config.OrganizationSlug()
			var builds []buildkite.Build

			err = io.SpinWhile("Loading builds", func() {
				builds, err = fetchBuilds(cmd.Context(), f, org, opts, listOpts)
			})
			if err != nil {
				return fmt.Errorf("failed to list builds: %w", err)
			}

			if opts.duration != "" || opts.message != "" {
				builds, err = applyClientSideFilters(builds, opts)
				if err != nil {
					return fmt.Errorf("failed to apply filters: %w", err)
				}
			}

			if len(builds) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No builds found matching the specified criteria.")
				return nil
			}

			return displayBuilds(cmd, builds, format)
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.ReadBuilds),
	}

	cmd.Flags().StringVarP(&opts.pipeline, "pipeline", "p", "", "Filter by pipeline slug")
	cmd.Flags().StringVar(&opts.since, "since", "", "Filter builds created since this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.until, "until", "", "Filter builds created before this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.duration, "duration", "", "Filter by duration (e.g. >5m, <10m, 20m)")
	cmd.Flags().StringSliceVar(&opts.state, "state", []string{}, "Filter by build state")
	cmd.Flags().StringSliceVar(&opts.branch, "branch", []string{}, "Filter by branch name")
	cmd.Flags().StringVar(&opts.creator, "creator", "", "Filter by creator")
	cmd.Flags().StringVar(&opts.commit, "commit", "", "Filter by commit SHA")
	cmd.Flags().StringVar(&opts.message, "message", "", "Filter by message content")
	cmd.Flags().IntVar(&opts.limit, "limit", 50, fmt.Sprintf("Maximum number of builds to return (max: %d)", maxBuildLimit))

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false

	return &cmd
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

func fetchBuilds(ctx context.Context, f *factory.Factory, org string, opts buildListOptions, listOpts *buildkite.BuildsListOptions) ([]buildkite.Build, error) {
	var allBuilds []buildkite.Build

	for page := 1; len(allBuilds) < opts.limit; page++ {
		listOpts.Page = page
		listOpts.PerPage = min(pageSize, opts.limit-len(allBuilds))

		var builds []buildkite.Build
		var err error

		if opts.pipeline != "" {
			builds, err = getBuildsByPipeline(ctx, f, org, opts.pipeline, listOpts)
		} else {
			builds, _, err = f.RestAPIClient.Builds.ListByOrg(ctx, org, listOpts)
		}

		if err != nil {
			return nil, err
		}

		if len(builds) == 0 {
			break
		}

		allBuilds = append(allBuilds, builds...)

		if len(builds) < listOpts.PerPage {
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

func displayBuilds(cmd *cobra.Command, builds []buildkite.Build, format output.Format) error {
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
