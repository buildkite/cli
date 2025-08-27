package build

import (
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
			# List recent builds
			$ bk build list

			# List builds from the last hour
			$ bk build list --since 1h (s, m, h)

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

			listOpts, err := buildListOptionsFromFlags(&opts)
			if err != nil {
				return err
			}

			org := f.Config.OrganizationSlug()
			var builds []buildkite.Build

			if opts.pipeline != "" {
				pipelineRes := pipelineResolver.NewAggregateResolver(
					pipelineResolver.ResolveFromFlag(opts.pipeline, f.Config),
					pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				)

				pipeline, err := pipelineRes.Resolve(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to resolve pipeline: %w", err)
				}

				err = io.SpinWhile("Loading builds", func() {
					builds, _, err = f.RestAPIClient.Builds.ListByPipeline(
						cmd.Context(),
						org,
						pipeline.Name,
						listOpts,
					)
				})
				if err != nil {
					return fmt.Errorf("failed to list builds: %w", err)
				}
			} else {
				err = io.SpinWhile("Loading builds", func() {
					builds, _, err = f.RestAPIClient.Builds.ListByOrg(cmd.Context(), org, listOpts)
				})
				if err != nil {
					return fmt.Errorf("failed to list builds: %w", err)
				}
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
	cmd.Flags().IntVar(&opts.limit, "limit", 50, "Maximum number of builds to return")

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false

	return &cmd
}

func buildListOptionsFromFlags(opts *buildListOptions) (*buildkite.BuildsListOptions, error) {
	listOpts := &buildkite.BuildsListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: opts.limit,
		},
	}

	if opts.since != "" {
		duration, err := parseDuration(opts.since)
		if err != nil {
			return nil, fmt.Errorf("invalid since duration '%s': %w", opts.since, err)
		}
		listOpts.CreatedFrom = time.Now().Add(-duration)
	}

	if opts.until != "" {
		duration, err := parseDuration(opts.until)
		if err != nil {
			return nil, fmt.Errorf("invalid until duration '%s': %w", opts.until, err)
		}
		listOpts.CreatedTo = time.Now().Add(-duration)
	}

	if len(opts.state) > 0 {
		listOpts.State = opts.state
	}

	if len(opts.branch) > 0 {
		listOpts.Branch = opts.branch
	}

	if opts.creator != "" {
		listOpts.Creator = opts.creator
	}

	if opts.commit != "" {
		listOpts.Commit = opts.commit
	}

	return listOpts, nil
}

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

func applyClientSideFilters(builds []buildkite.Build, opts buildListOptions) ([]buildkite.Build, error) {
	if opts.duration == "" && opts.message == "" {
		return builds, nil
	}

	var durationFilter struct {
		op       string
		duration time.Duration
	}

	if opts.duration != "" {
		op, durStr := ">= ", opts.duration
		if strings.HasPrefix(opts.duration, "<") {
			op, durStr = "<", opts.duration[1:]
		} else if strings.HasPrefix(opts.duration, ">") {
			op, durStr = ">", opts.duration[1:]
		}

		d, err := time.ParseDuration(durStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format: %w", err)
		}
		durationFilter.op = op
		durationFilter.duration = d
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
				elapsed = build.FinishedAt.Time.Sub(build.StartedAt.Time)
			} else {
				elapsed = time.Since(build.StartedAt.Time)
			}

			switch durationFilter.op {
			case "<":
				if elapsed >= durationFilter.duration {
					continue
				}
			case ">":
				if elapsed <= durationFilter.duration {
					continue
				}
			default: // ">="
				if elapsed < durationFilter.duration {
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
		tableWidth       = 120
		timeFormat       = "2006-01-02T15:04:05Z"
	)

	var buf strings.Builder

	header := lipgloss.NewStyle().Bold(true).Underline(true).Render("Builds")
	buf.WriteString(header)
	buf.WriteString("\n\n")

	headerRow := fmt.Sprintf("%-8s %-12s %-25s %-20s %-20s %-12s %s",
		"Number", "State", "Message", "Started (UTC)", "Finished (UTC)", "Duration", "URL")
	buf.WriteString(lipgloss.NewStyle().Bold(true).Render(headerRow))
	buf.WriteString("\n")
	buf.WriteString(strings.Repeat("-", tableWidth))
	buf.WriteString("\n")

	for _, build := range builds {
		message := build.Message
		if len(message) > maxMessageLength {
			message = message[:truncatedLength] + "..."
		}

		startedAt := "-"
		if build.StartedAt != nil {
			startedAt = build.StartedAt.Time.Format(timeFormat)
		}

		finishedAt := "-"
		duration := "-"
		if build.FinishedAt != nil {
			finishedAt = build.FinishedAt.Time.Format(timeFormat)
			if build.StartedAt != nil {
				dur := build.FinishedAt.Time.Sub(build.StartedAt.Time)
				duration = formatDuration(dur)
			}
		} else if build.StartedAt != nil {
			dur := time.Since(build.StartedAt.Time)
			duration = formatDuration(dur) + " (running)"
		}

		stateColor := getStateColor(build.State)
		coloredState := stateColor.Render(build.State)

		row := fmt.Sprintf("%-8d %-12s %-25s %-20s %-20s %-12s %s",
			build.Number, coloredState, message, startedAt, finishedAt, duration, build.WebURL)
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
