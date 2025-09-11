package job

import (
	"context"
	"fmt"
	"sort"
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
	maxJobLimit = 5000
	pageSize    = 100
)

type jobListOptions struct {
	pipeline string
	since    string
	until    string
	duration string
	state    []string
	queue    string
	orderBy  string
	limit    int
}

func (opts jobListOptions) withoutQueue() jobListOptions {
	newOpts := opts
	newOpts.queue = ""
	return newOpts
}

func NewCmdJobList(f *factory.Factory) *cobra.Command {
	var opts jobListOptions

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [flags]",
		Short:                 "List jobs",
		Long: heredoc.Doc(`
			List jobs with optional filtering.

			This command supports both server-side filtering (fast) and client-side filtering.
			Server-side filters are applied when fetching builds, while client-side filters
			are applied after extracting jobs from builds.

			Client-side filters: --queue, --state, --duration
			Server-side filters: --pipeline, --since, --until

			Jobs can be filtered by queue, state, duration, and other attributes.
			When filtering by duration, you can use operators like >, <, >=, and <= to specify your criteria.
			Supported duration units are seconds (s), minutes (m), and hours (h).
		`),
		Example: heredoc.Doc(`
			# List recent jobs (100 by default)
			$ bk job list

			# List jobs from a specific queue
			$ bk job list --queue test-queue

			# List running jobs
			$ bk job list --state running

			# List jobs that took longer than 10 minutes
			$ bk job list --duration ">10m"

			# List jobs from the last hour
			$ bk job list --since 1h

			# Combine filters
			$ bk job list --queue test-queue --state running --duration ">10m"

			# Order by duration (longest first)
			$ bk job list --order-by duration

			# Get JSON output for bulk operations
			$ bk job list --queue test-queue -o json
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

			if opts.limit > maxJobLimit {
				return fmt.Errorf("limit cannot exceed %d jobs (requested: %d)", maxJobLimit, opts.limit)
			}

			listOpts, err := jobListOptionsFromFlags(&opts)
			if err != nil {
				return err
			}

			org := f.Config.OrganizationSlug()
			var jobs []buildkite.Job

			err = io.SpinWhile("Loading jobs", func() {
				if opts.queue != "" {
					jobs, err = fetchJobsWithQueueFilter(cmd.Context(), f, org, opts)
				} else {
					jobs, err = fetchJobs(cmd.Context(), f, org, opts, listOpts)
				}
			})
			if err != nil {
				return fmt.Errorf("failed to list jobs: %w", err)
			}

			if opts.queue == "" && (len(opts.state) > 0 || opts.duration != "") {
				jobs, err = applyClientSideFilters(jobs, opts)
				if err != nil {
					return fmt.Errorf("failed to apply filters: %w", err)
				}
			}

			if opts.orderBy != "" {
				jobs = sortJobs(jobs, opts.orderBy)
			}

			if len(jobs) > opts.limit {
				jobs = jobs[:opts.limit]
			}

			if len(jobs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No jobs found matching the specified criteria.")
				return nil
			}

			return displayJobs(cmd, jobs, format)
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.ReadBuilds),
	}

	cmd.Flags().StringVarP(&opts.pipeline, "pipeline", "p", "", "Filter by pipeline slug")
	cmd.Flags().StringVar(&opts.since, "since", "", "Filter jobs from builds created since this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.until, "until", "", "Filter jobs from builds created before this time (e.g. 1h, 30m)")
	cmd.Flags().StringVar(&opts.duration, "duration", "", "Filter by duration (e.g. >10m, <5m, 20m) - supports >, <, >=, <= operators")
	cmd.Flags().StringSliceVar(&opts.state, "state", []string{}, "Filter by job state")
	cmd.Flags().StringVar(&opts.queue, "queue", "", "Filter by queue name")
	cmd.Flags().StringVar(&opts.orderBy, "order-by", "", "Order results by field (start_time, duration)")
	cmd.Flags().IntVar(&opts.limit, "limit", 100, fmt.Sprintf("Maximum number of jobs to return (max: %d)", maxJobLimit))

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false

	return &cmd
}

func fetchJobs(ctx context.Context, f *factory.Factory, org string, opts jobListOptions, listOpts *buildkite.BuildsListOptions) ([]buildkite.Job, error) {
	allJobs := make([]buildkite.Job, 0, opts.limit*2)
	maxBuildsToFetch := min(200, opts.limit*2)

	maxPages := (maxBuildsToFetch + pageSize - 1) / pageSize
	for page := 1; len(allJobs) < opts.limit*2 && page <= maxPages; page++ {
		listOpts.Page = page
		remaining := maxBuildsToFetch - ((page - 1) * pageSize)
		listOpts.PerPage = min(pageSize, remaining)

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

		for _, build := range builds {
			if len(allJobs)+len(build.Jobs) > cap(allJobs) {
				newJobs := make([]buildkite.Job, len(allJobs), len(allJobs)+len(build.Jobs)+100)
				copy(newJobs, allJobs)
				allJobs = newJobs
			}
			allJobs = append(allJobs, build.Jobs...)
		}

		if len(allJobs) >= opts.limit*2 {
			break
		}

		if len(builds) < listOpts.PerPage {
			break
		}
	}

	return allJobs, nil
}

func fetchJobsWithQueueFilter(ctx context.Context, f *factory.Factory, org string, opts jobListOptions) ([]buildkite.Job, error) {
	queueIDs, err := lookupQueueIDs(ctx, f, org, opts.queue)
	if err != nil {
		return nil, err
	}
	if len(queueIDs) == 0 {
		return []buildkite.Job{}, nil
	}

	var jobs []buildkite.Job
	var cursor *string
	noQueueOpts := opts.withoutQueue()

	for len(jobs) < opts.limit {
		jobBatch, nextCursor, hasNext, err := listJobsByQueue(ctx, f, org, queueIDs, cursor)
		if err != nil {
			return nil, err
		}
		if len(jobBatch) == 0 {
			break
		}

		if len(noQueueOpts.state) > 0 || noQueueOpts.duration != "" {
			jobBatch, err = applyClientSideFilters(jobBatch, noQueueOpts)
			if err != nil {
				return nil, fmt.Errorf("failed to apply filters: %w", err)
			}
		}

		for _, job := range jobBatch {
			if len(jobs) >= opts.limit {
				break
			}
			jobs = append(jobs, job)
		}

		if !hasNext {
			break
		}
		cursor = nextCursor
	}

	return jobs, nil
}

func lookupQueueIDs(ctx context.Context, f *factory.Factory, org, queueName string) ([]string, error) {
	resp, err := graphql.FindQueues(ctx, f.GraphQLClient, org)
	if err != nil {
		return nil, fmt.Errorf("failed to find queues: %w", err)
	}
	return findMatchingQueues(resp, queueName), nil
}

func listJobsByQueue(ctx context.Context, f *factory.Factory, org string, queueIDs []string, cursor *string) ([]buildkite.Job, *string, bool, error) {
	first := pageSize
	resp, err := graphql.ListJobsByQueue(ctx, f.GraphQLClient, org, queueIDs, &first, cursor)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to list jobs: %w", err)
	}

	if resp.Organization == nil || resp.Organization.Jobs == nil {
		return []buildkite.Job{}, nil, false, nil
	}

	var jobs []buildkite.Job
	for _, edge := range resp.Organization.Jobs.Edges {
		if edge.Node != nil {
			jobs = append(jobs, convertGraphQLJobToBuildkiteJob(edge.Node))
		}
	}

	hasMore := resp.Organization.Jobs.PageInfo != nil && resp.Organization.Jobs.PageInfo.HasNextPage
	nextCursor := (*string)(nil)
	if hasMore && resp.Organization.Jobs.PageInfo.EndCursor != nil {
		nextCursor = resp.Organization.Jobs.PageInfo.EndCursor
	}

	return jobs, nextCursor, hasMore, nil
}

func findMatchingQueues(queuesResp *graphql.FindQueuesResponse, targetQueue string) []string {
	if queuesResp.Organization == nil || queuesResp.Organization.Clusters == nil {
		return nil
	}

	var matchingIDs []string
	targetLower := strings.ToLower(targetQueue)

	for _, clusterEdge := range queuesResp.Organization.Clusters.Edges {
		if clusterEdge.Node == nil || clusterEdge.Node.Queues == nil {
			continue
		}

		for _, queueEdge := range clusterEdge.Node.Queues.Edges {
			if queueEdge.Node != nil && strings.ToLower(queueEdge.Node.Key) == targetLower {
				matchingIDs = append(matchingIDs, queueEdge.Node.Id)
			}
		}
	}

	return matchingIDs
}

func convertGraphQLJobToBuildkiteJob(jobNode *graphql.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJob) buildkite.Job {
	// Handle the union type - we only care about JobTypeCommand for now
	switch job := (*jobNode).(type) {
	case *graphql.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJobTypeCommand:
		startedAt := convertTimestamp(job.StartedAt)
		finishedAt := convertTimestamp(job.FinishedAt)
		createdAt := convertTimestamp(job.CreatedAt)
		agent := convertAgent(job.Agent)

		// Build label (jobs don't have labels in GraphQL, so we use command or empty)
		label := derefString(job.Command)

		return buildkite.Job{
			ID:              job.Id,
			Type:            "script",
			Name:            job.Uuid, // Use UUID as name
			Label:           label,
			Command:         derefString(job.Command),
			State:           mapGraphQLState(string(job.State), derefString(job.ExitStatus)),
			WebURL:          job.Url,
			StartedAt:       startedAt,
			FinishedAt:      finishedAt,
			CreatedAt:       createdAt,
			Agent:           agent,
			AgentQueryRules: []string{}, // Empty for GraphQL jobs
		}
	default:
		// For non-command jobs, return a minimal job struct
		return buildkite.Job{
			ID:    "unknown",
			Type:  "unknown",
			State: "unknown",
		}
	}
}

func convertTimestamp(t *time.Time) *buildkite.Timestamp {
	if t == nil {
		return nil
	}
	return &buildkite.Timestamp{Time: *t}
}

func convertAgent(agentNode *graphql.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJobTypeCommandAgent) buildkite.Agent {
	if agentNode == nil {
		return buildkite.Agent{}
	}

	return buildkite.Agent{
		ID:       agentNode.Id,
		Name:     agentNode.Name,
		Hostname: derefString(agentNode.Hostname),
		Metadata: agentNode.MetaData,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mapGraphQLState converts GraphQL job states to REST API equivalent states
func mapGraphQLState(graphqlState, exitStatus string) string {
	switch graphqlState {
	case "FINISHED":
		// For finished jobs, determine success/failure based on exit status
		if exitStatus == "0" {
			return "passed"
		}
		return "failed"
	case "RUNNING":
		return "running"
	case "SCHEDULED", "ASSIGNED", "ACCEPTED":
		return "scheduled"
	case "CANCELED", "CANCELING":
		return "canceled"
	case "TIMED_OUT", "TIMING_OUT":
		return "timed_out"
	case "SKIPPED":
		return "skipped"
	case "BLOCKED":
		return "blocked"
	case "WAITING":
		return "waiting"
	default:
		// For unknown states, return lowercase version of GraphQL state
		return strings.ToLower(graphqlState)
	}
}

func jobListOptionsFromFlags(opts *jobListOptions) (*buildkite.BuildsListOptions, error) {
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

	return listOpts, nil
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

func applyClientSideFilters(jobs []buildkite.Job, opts jobListOptions) ([]buildkite.Job, error) {
	if opts.queue == "" && len(opts.state) == 0 && opts.duration == "" {
		return jobs, nil
	}

	var durationOp string
	var durationThreshold time.Duration
	var normalizedStates []string

	if len(opts.state) > 0 {
		normalizedStates = make([]string, len(opts.state))
		for i, state := range opts.state {
			normalizedStates[i] = strings.ToLower(state)
		}
	}

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

	result := make([]buildkite.Job, 0, len(jobs)/2)

	for i := range jobs {
		job := &jobs[i]

		if opts.queue != "" {
			if !matchesQueue(*job, opts.queue) {
				continue
			}
		}

		if len(normalizedStates) > 0 {
			if !containsString(normalizedStates, strings.ToLower(job.State)) {
				continue
			}
		}

		if opts.duration != "" {
			if job.StartedAt == nil {
				continue
			}

			var elapsed time.Duration
			if job.FinishedAt != nil {
				elapsed = job.FinishedAt.Sub(job.StartedAt.Time)
			} else {
				elapsed = time.Since(job.StartedAt.Time)
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

		result = append(result, *job)
	}

	return result, nil
}

func matchesQueue(job buildkite.Job, queueFilter string) bool {
	for _, rule := range job.AgentQueryRules {
		if strings.Contains(strings.ToLower(rule), "queue="+strings.ToLower(queueFilter)) {
			return true
		}
		if strings.EqualFold(rule, queueFilter) {
			return true
		}
	}

	for _, meta := range job.Agent.Metadata {
		if strings.Contains(strings.ToLower(meta), "queue="+strings.ToLower(queueFilter)) {
			return true
		}
		if strings.EqualFold(meta, queueFilter) {
			return true
		}
	}

	return false
}

func sortJobs(jobs []buildkite.Job, orderBy string) []buildkite.Job {
	if orderBy == "" {
		return jobs
	}

	sort.Slice(jobs, func(i, j int) bool {
		switch orderBy {
		case "start_time":
			if jobs[i].StartedAt == nil && jobs[j].StartedAt == nil {
				return false
			}
			if jobs[i].StartedAt == nil {
				return false
			}
			if jobs[j].StartedAt == nil {
				return true
			}
			return jobs[i].StartedAt.Before(jobs[j].StartedAt.Time)
		case "duration":
			durI := getJobDuration(jobs[i])
			durJ := getJobDuration(jobs[j])
			return durI > durJ
		default:
			return false
		}
	})

	return jobs
}

func getJobDuration(job buildkite.Job) time.Duration {
	if job.StartedAt == nil {
		return 0
	}
	if job.FinishedAt != nil {
		return job.FinishedAt.Sub(job.StartedAt.Time)
	}
	return time.Since(job.StartedAt.Time)
}

func displayJobs(cmd *cobra.Command, jobs []buildkite.Job, format output.Format) error {
	if format != output.FormatText {
		return output.Write(cmd.OutOrStdout(), jobs, format)
	}

	const (
		maxLabelLength  = 35
		truncatedLength = 32
		timeFormat      = "2006-01-02T15:04:05Z"
		stateWidth      = 12
		labelWidth      = 38
		timeWidth       = 20
		durationWidth   = 12
		columnSpacing   = 6
	)

	var buf strings.Builder

	header := lipgloss.NewStyle().Bold(true).Underline(true).Render("Jobs")
	buf.WriteString(header)
	buf.WriteString("\n\n")

	headerRow := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %s",
		stateWidth, "State",
		labelWidth, "Label",
		timeWidth, "Started (UTC)",
		timeWidth, "Finished (UTC)",
		durationWidth, "Duration",
		"URL")
	buf.WriteString(lipgloss.NewStyle().Bold(true).Render(headerRow))
	buf.WriteString("\n")
	totalWidth := stateWidth + labelWidth + timeWidth*2 + durationWidth + columnSpacing
	buf.WriteString(strings.Repeat("-", totalWidth))
	buf.WriteString("\n")

	for _, job := range jobs {
		label := job.Label
		if label == "" {
			label = job.Name
		}
		if len(label) > maxLabelLength {
			label = label[:truncatedLength] + "..."
		}

		startedAt := "-"
		if job.StartedAt != nil {
			startedAt = job.StartedAt.Format(timeFormat)
		}

		finishedAt := "-"
		duration := "-"
		if job.FinishedAt != nil {
			finishedAt = job.FinishedAt.Format(timeFormat)
			if job.StartedAt != nil {
				dur := job.FinishedAt.Sub(job.StartedAt.Time)
				duration = formatDuration(dur)
			}
		} else if job.StartedAt != nil {
			dur := time.Since(job.StartedAt.Time)
			duration = formatDuration(dur) + " (running)"
		}

		stateColor := getJobStateColor(job.State)
		coloredState := stateColor.Render(job.State)

		row := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %s",
			stateWidth, coloredState,
			labelWidth, label,
			timeWidth, startedAt,
			timeWidth, finishedAt,
			durationWidth, duration,
			job.WebURL)
		buf.WriteString(row)
		buf.WriteString("\n")
	}

	_, err := cmd.OutOrStdout().Write([]byte(buf.String()))
	return err
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

func getJobStateColor(state string) lipgloss.Style {
	switch strings.ToLower(state) {
	case "passed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // Green
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // Red
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // Yellow
	case "scheduled", "waiting":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // Cyan
	case "canceled", "cancelled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray
	case "blocked":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // Magenta
	default:
		return lipgloss.NewStyle()
	}
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
