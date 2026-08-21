package job

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

const (
	maxJobLimit = 5000
	pageSize    = 100
	// Pages to walk while a filter the server cannot apply is discarding results.
	maxClientFilterPages = 20
)

type ListCmd struct {
	Pipeline string   `help:"Filter by pipeline slug" short:"p"`
	Build    string   `help:"Filter by build number (requires a resolvable pipeline)"`
	StepKey  string   `help:"Filter by step key (requires --build)" name:"step-key"`
	GroupKey string   `help:"Filter by group key (requires --build)" name:"group-key"`
	Since    string   `help:"Filter jobs from builds created since this time (e.g. 1h, 30m)"`
	Until    string   `help:"Filter jobs from builds created before this time (e.g. 1h, 30m)"`
	Duration string   `help:"Filter by duration (e.g. >10m, <5m, 20m) - supports >, <, >=, <= operators"`
	State    []string `help:"Filter by job state"`
	Queue    string   `help:"Filter by queue name"`
	OrderBy  string   `help:"Order results by field (start_time, duration)" name:"order-by"`
	Limit    int      `help:"Maximum number of jobs to return" default:"100"`
	NoLimit  bool     `help:"Fetch all jobs (overrides --limit)" name:"no-limit"`
	output.OutputFlags
}

func (c *ListCmd) Help() string {
	return `This command supports both server-side filtering (fast) and client-side filtering.
When a build number is known, use --build to fetch its jobs directly. The pipeline
can be passed with --pipeline or resolved from the current repository or config.

Server-side filters: --pipeline, --build, --step-key, --group-key, --since,
--until, --queue, --state

Client-side filters: --duration, and --state without --queue or --build

Without --build, the command fetches up to 200 builds for filtering by default.
Use --no-limit to search further.

Jobs can be filtered by queue, state, duration, and other attributes.
When filtering by duration, you can use operators like >, <, >=, and <= to specify your criteria.
Supported duration units are seconds (s), minutes (m), and hours (h).

Examples:
  # List recent jobs (100 by default)
  $ bk job list

  # List jobs from a specific queue
  $ bk job list --queue test-queue

  # List running jobs in a queue (both filters applied by the server)
  $ bk job list --queue test-queue --state running

  # List running jobs
  $ bk job list --state running

  # List failed jobs from a known build (recommended when the build is known)
  $ bk job list --pipeline my-app --build 429 --state failed

  # List jobs for step and group keys from a known build
  $ bk job list --pipeline my-app --build 429 --step-key test --group-key verification

  # List jobs that took longer than 10 minutes
  $ bk job list --duration ">10m"

  # List jobs from the last hour
  $ bk job list --since 1h

  # Combine filters
  $ bk job list --queue test-queue --state running --duration ">10m"

  # Fetch all jobs matching filters (no limit)
  $ bk job list --duration ">10m" --no-limit

  # Order by duration (longest first)
  $ bk job list --order-by duration

  # Get JSON output for bulk operations
  $ bk job list --queue test-queue -o json
`
}

type jobListOptions struct {
	pipeline string
	build    string
	stepKey  string
	groupKey string
	since    string
	until    string
	duration string
	state    []string
	queue    string
	orderBy  string
	limit    int
	noLimit  bool
}

func (opts jobListOptions) withoutQueue() jobListOptions {
	newOpts := opts
	newOpts.queue = ""
	return newOpts
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	if !c.NoLimit && c.Limit > maxJobLimit {
		return fmt.Errorf("limit cannot exceed %d jobs (requested: %d); if you need more, use --no-limit", maxJobLimit, c.Limit)
	}

	opts := jobListOptions{
		pipeline: c.Pipeline,
		build:    c.Build,
		stepKey:  c.StepKey,
		groupKey: c.GroupKey,
		since:    c.Since,
		until:    c.Until,
		duration: c.Duration,
		state:    c.State,
		queue:    c.Queue,
		orderBy:  c.OrderBy,
		limit:    c.Limit,
		noLimit:  c.NoLimit,
	}

	listOpts, err := jobListOptionsFromFlags(&opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	org := f.Config.OrganizationSlug()
	var jobs []buildkite.Job
	var resolvedPipeline *pipeline.Pipeline
	var queueIDs []string

	if err = bkIO.SpinWhile(f, "Loading jobs", func() error {
		jobs, resolvedPipeline, queueIDs, err = fetchJobList(ctx, f, org, opts, listOpts)
		return err
	}); err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	shouldApplyFilters := opts.build != "" || opts.queue == ""
	if shouldApplyFilters && (opts.queue != "" || len(opts.state) > 0 || opts.duration != "") {
		jobs, err = applyClientSideFilters(jobs, opts, queueIDs)
		if err != nil {
			return fmt.Errorf("failed to apply filters: %w", err)
		}
	}

	if opts.orderBy != "" {
		jobs = sortJobs(jobs, opts.orderBy)
	}

	// Apply limit only if --no-limit is not set
	if !opts.noLimit && len(jobs) > opts.limit {
		jobs = jobs[:opts.limit]
	}

	if len(jobs) == 0 {
		if format != output.FormatText {
			return output.Write(os.Stdout, []buildkite.Job{}, format)
		}
		fmt.Println("No jobs found matching the specified criteria.")
		return nil
	}

	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
		defer func() { _ = cleanup() }()

		target := org
		if resolvedPipeline != nil {
			target = fmt.Sprintf("%s/%s", resolvedPipeline.Org, resolvedPipeline.Name)
		} else if c.Pipeline != "" {
			target = fmt.Sprintf("%s/%s", org, c.Pipeline)
		}

		fmt.Fprintf(writer, "Showing %d jobs for %s\n\n", len(jobs), target)
		return displayJobs(jobs, format, writer)
	}

	return displayJobs(jobs, format, os.Stdout)
}

func fetchJobList(ctx context.Context, f *factory.Factory, org string, opts jobListOptions, listOpts *buildkite.BuildsListOptions) ([]buildkite.Job, *pipeline.Pipeline, []string, error) {
	if opts.build != "" {
		resolvedPipeline, err := resolveJobListPipeline(ctx, f, opts.pipeline)
		if err != nil {
			return nil, nil, nil, err
		}

		var queueIDs []string
		if opts.queue != "" {
			queueIDs, err = lookupQueueIDs(ctx, f, resolvedPipeline.Org, opts.queue)
			if err != nil {
				return nil, nil, nil, err
			}
		}

		fetchAll := opts.noLimit || opts.queue != "" || opts.duration != "" || opts.orderBy != ""
		jobs, err := fetchJobsByBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, opts.build, opts, fetchAll)
		return jobs, resolvedPipeline, queueIDs, err
	}

	if opts.queue != "" {
		jobs, err := fetchJobsWithQueueFilter(ctx, f, org, opts)
		return jobs, nil, nil, err
	}

	jobs, err := fetchJobs(ctx, f, org, opts, listOpts)
	return jobs, nil, nil, err
}

func resolveJobListPipeline(ctx context.Context, f *factory.Factory, pipelineFlag string) (*pipeline.Pipeline, error) {
	resolvers := pipelineResolver.AggregateResolver{
		pipelineResolver.ResolveFromFlag(pipelineFlag, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	}

	resolvedPipeline, err := resolvers.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("--build requires a pipeline; specify one with --pipeline or run from a linked repository: %w", err)
	}
	if resolvedPipeline == nil {
		return nil, fmt.Errorf("--build requires a pipeline; specify one with --pipeline or run from a linked repository")
	}

	return resolvedPipeline, nil
}

func fetchJobsByBuild(ctx context.Context, client *buildkite.Client, org, pipeline, build string, opts jobListOptions, fetchAll bool) ([]buildkite.Job, error) {
	fetchAll = fetchAll || opts.noLimit
	if !fetchAll && opts.limit <= 0 {
		return []buildkite.Job{}, nil
	}

	includeRetriedJobs := false
	jobsListOpts := &buildkite.JobsListOptions{
		State:              opts.state,
		StepKey:            opts.stepKey,
		GroupKey:           opts.groupKey,
		IncludeRetriedJobs: &includeRetriedJobs,
		PerPage:            pageSize,
	}
	if !fetchAll {
		jobsListOpts.PerPage = min(opts.limit, pageSize)
	}

	var jobs []buildkite.Job
	for {
		page, _, err := client.Jobs.ListByBuild(ctx, org, pipeline, build, jobsListOpts)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, page.Items...)

		if !fetchAll && len(jobs) >= opts.limit {
			return jobs[:opts.limit], nil
		}
		if page.Links.Next == "" {
			return jobs, nil
		}

		jobsListOpts, err = page.Links.Next.ToOptions()
		if err != nil {
			return nil, fmt.Errorf("invalid next jobs page: %w", err)
		}
		jobsListOpts.State = opts.state
		jobsListOpts.StepKey = opts.stepKey
		jobsListOpts.GroupKey = opts.groupKey
		jobsListOpts.IncludeRetriedJobs = &includeRetriedJobs
		jobsListOpts.PerPage = pageSize
		if !fetchAll {
			jobsListOpts.PerPage = min(opts.limit-len(jobs), pageSize)
		}
	}
}

func fetchJobs(ctx context.Context, f *factory.Factory, org string, opts jobListOptions, listOpts *buildkite.BuildsListOptions) ([]buildkite.Job, error) {
	var maxBuildsToFetch int
	if opts.noLimit {
		// When --no-limit is set, fetch all available builds (no upper bound)
		maxBuildsToFetch = 0 // 0 means unlimited
	} else {
		// By default, fetch a reasonable number of builds (200 = 2 pages)
		// This provides a good pool for filtering without being tied to --limit
		maxBuildsToFetch = 200
	}

	allJobs := make([]buildkite.Job, 0, opts.limit*2)
	buildsFetched := 0

	// Calculate max pages (0 means unlimited)
	var maxPages int
	if maxBuildsToFetch > 0 {
		maxPages = (maxBuildsToFetch + pageSize - 1) / pageSize
	}

	for page := 1; ; page++ {
		// Check page limit if set
		if maxPages > 0 && page > maxPages {
			break
		}
		listOpts.Page = page
		listOpts.PerPage = pageSize

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

		buildsFetched += len(builds)

		for _, build := range builds {
			if len(allJobs)+len(build.Jobs) > cap(allJobs) {
				newJobs := make([]buildkite.Job, len(allJobs), len(allJobs)+len(build.Jobs)+100)
				copy(newJobs, allJobs)
				allJobs = newJobs
			}
			allJobs = append(allJobs, build.Jobs...)
		}

		// Stop if we got fewer builds than requested (last page)
		if len(builds) < pageSize {
			break
		}

		// Stop if we've reached the maximum builds to fetch (only when limit is set)
		if maxBuildsToFetch > 0 && buildsFetched >= maxBuildsToFetch {
			break
		}
	}

	return allJobs, nil
}

type listJobsByQueue func(ctx context.Context, f *factory.Factory, org string, queueIDs []string, states []bkGraphQL.JobStates, cursor *string) ([]buildkite.Job, *string, bool, error)

func listJobsWithPagination(ctx context.Context, f *factory.Factory, org string, queueIDs []string, opts jobListOptions, listJobs listJobsByQueue) ([]buildkite.Job, error) {
	var jobs []buildkite.Job
	var cursor *string
	states, exactStates := graphQLStatesFor(opts.state)

	clientOpts := opts.withoutQueue()
	if exactStates {
		clientOpts.state = nil
	}

	// Without a bound, a filter matching fewer than --limit jobs walks the
	// queue's entire history.
	pagesRemaining := maxClientFilterPages
	boundPages := !opts.noLimit && (len(clientOpts.state) > 0 || clientOpts.duration != "")

	for len(jobs) < opts.limit {
		jobBatch, nextCursor, hasNext, err := listJobs(ctx, f, org, queueIDs, states, cursor)
		if err != nil {
			return nil, err
		}
		if len(jobBatch) == 0 {
			break
		}

		// Apply client-side filters if needed
		if len(clientOpts.state) > 0 || clientOpts.duration != "" {
			jobBatch, err = applyClientSideFilters(jobBatch, clientOpts, nil)
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
		if boundPages {
			if pagesRemaining--; pagesRemaining <= 0 {
				fmt.Fprintf(os.Stderr, "Stopped after searching %d jobs. Narrow the search, or use --no-limit to keep going.\n", maxClientFilterPages*pageSize)
				break
			}
		}
		cursor = nextCursor
	}

	return jobs, nil
}

func fetchJobsWithQueueFilter(ctx context.Context, f *factory.Factory, org string, opts jobListOptions) ([]buildkite.Job, error) {
	queueIDs, err := lookupQueueIDs(ctx, f, org, opts.queue)
	if err != nil {
		return nil, err
	}

	if len(queueIDs) == 0 {
		// Fallback to unclustered agent query rules
		agentQueryRules := []string{"queue=" + strings.ToLower(opts.queue)}
		return listJobsWithPagination(ctx, f, org, agentQueryRules, opts, listJobsByAgentQueryRules)
	}

	return listJobsWithPagination(ctx, f, org, queueIDs, opts, listJobsByClusterQueue)
}

const maxConcurrentRequests = 10 // Balance between performance and API rate limits

type ClusterInfo struct {
	ID   string
	Name string
}

func lookupQueueIDs(ctx context.Context, f *factory.Factory, org, queueName string) ([]string, error) {
	clusters, err := fetchAllClusters(ctx, f.GraphQLClient, org)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clusters: %w", err)
	}

	if len(clusters) == 0 {
		return []string{}, nil
	}

	return fetchQueuesFromClusters(ctx, f.GraphQLClient, clusters, queueName)
}

func fetchAllClusters(ctx context.Context, client graphql.Client, org string) ([]ClusterInfo, error) {
	var allClusters []ClusterInfo
	var cursor *string

	for {
		resp, err := bkGraphQL.FindClusters(ctx, client, org, cursor)
		if err != nil {
			return nil, err
		}

		if resp.Organization == nil || resp.Organization.Clusters == nil {
			break
		}

		for _, edge := range resp.Organization.Clusters.Edges {
			if edge.Node != nil {
				allClusters = append(allClusters, ClusterInfo{
					ID:   edge.Node.Id,
					Name: edge.Node.Name,
				})
			}
		}

		if resp.Organization.Clusters.PageInfo != nil && resp.Organization.Clusters.PageInfo.HasNextPage {
			cursor = resp.Organization.Clusters.PageInfo.EndCursor
		} else {
			break
		}
	}

	return allClusters, nil
}

func fetchQueuesFromClusters(ctx context.Context, client graphql.Client, clusters []ClusterInfo, queueName string) ([]string, error) {
	resultChan := make(chan []string, len(clusters))
	errorChan := make(chan error, len(clusters))
	semaphore := make(chan struct{}, maxConcurrentRequests)

	var wg sync.WaitGroup

	for _, cluster := range clusters {
		wg.Add(1)
		go func(c ClusterInfo) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			queueIDs, err := fetchQueuesForCluster(ctx, client, c.ID, queueName)
			if err != nil {
				errorChan <- fmt.Errorf("cluster %s: %w", c.Name, err)
				return
			}

			resultChan <- queueIDs
		}(cluster)
	}

	var allQueueIDs []string
	var results int
	expectedResults := len(clusters)

	for results < expectedResults {
		select {
		case queueIDs := <-resultChan:
			allQueueIDs = append(allQueueIDs, queueIDs...)
			results++

		case err := <-errorChan:
			return nil, err

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return allQueueIDs, nil
}

func fetchQueuesForCluster(ctx context.Context, client graphql.Client, clusterID, queueName string) ([]string, error) {
	var matchingQueueIDs []string
	var cursor *string
	targetLower := strings.ToLower(queueName)

	for {
		resp, err := bkGraphQL.FindQueuesForCluster(ctx, client, clusterID, cursor)
		if err != nil {
			return nil, err
		}

		if resp.Node == nil {
			break
		}

		cluster, ok := (*resp.Node).(*bkGraphQL.FindQueuesForClusterNodeCluster)
		if !ok || cluster == nil || cluster.Queues == nil {
			break
		}

		for _, edge := range cluster.Queues.Edges {
			if edge.Node != nil && strings.ToLower(edge.Node.Key) == targetLower {
				matchingQueueIDs = append(matchingQueueIDs, edge.Node.Id)
			}
		}

		if cluster.Queues.PageInfo != nil && cluster.Queues.PageInfo.HasNextPage {
			cursor = cluster.Queues.PageInfo.EndCursor
		} else {
			break
		}
	}

	return matchingQueueIDs, nil
}

func listJobsByClusterQueue(ctx context.Context, f *factory.Factory, org string, queueIDs []string, states []bkGraphQL.JobStates, cursor *string) ([]buildkite.Job, *string, bool, error) {
	first := pageSize
	resp, err := bkGraphQL.ListJobsByQueue(ctx, f.GraphQLClient, org, queueIDs, states, &first, cursor)
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

func convertGraphQLJobToBuildkiteJob(jobNode *bkGraphQL.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJob) buildkite.Job {
	// Handle the union type - we only care about JobTypeCommand for now
	switch job := (*jobNode).(type) {
	case *bkGraphQL.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJobTypeCommand:
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

func listJobsByAgentQueryRules(ctx context.Context, f *factory.Factory, org string, agentQueryRules []string, states []bkGraphQL.JobStates, cursor *string) ([]buildkite.Job, *string, bool, error) {
	first := pageSize

	resp, err := bkGraphQL.ListJobsByAgentQueryRules(ctx, f.GraphQLClient, org, agentQueryRules, states, &first, cursor)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to list jobs: %w", err)
	}

	if resp.Organization == nil || resp.Organization.Jobs == nil {
		return []buildkite.Job{}, nil, false, nil
	}

	var jobs []buildkite.Job
	for _, edge := range resp.Organization.Jobs.Edges {
		if edge.Node != nil {
			jobs = append(jobs, convertGraphQLAgentQueryRulesJobToBuildkiteJob(edge.Node, agentQueryRules))
		}
	}

	hasMore := resp.Organization.Jobs.PageInfo != nil && resp.Organization.Jobs.PageInfo.HasNextPage
	nextCursor := (*string)(nil)
	if hasMore && resp.Organization.Jobs.PageInfo.EndCursor != nil {
		nextCursor = resp.Organization.Jobs.PageInfo.EndCursor
	}
	return jobs, nextCursor, hasMore, nil
}

func convertGraphQLAgentQueryRulesJobToBuildkiteJob(jobNode *bkGraphQL.ListJobsByAgentQueryRulesOrganizationJobsJobConnectionEdgesJobEdgeNodeJob, agentQueryRules []string) buildkite.Job {
	// Handle the union type - we only care about JobTypeCommand for now
	var agent buildkite.Agent
	switch job := (*jobNode).(type) {
	case *bkGraphQL.ListJobsByAgentQueryRulesOrganizationJobsJobConnectionEdgesJobEdgeNodeJobTypeCommand:
		startedAt := convertTimestamp(job.StartedAt)
		finishedAt := convertTimestamp(job.FinishedAt)
		createdAt := convertTimestamp(job.CreatedAt)

		if job.Agent != nil {
			agent = buildkite.Agent{
				ID:       job.Agent.Id,
				Name:     job.Agent.Name,
				Hostname: derefString(job.Agent.Hostname),
				Metadata: job.Agent.MetaData,
			}
		}

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
			AgentQueryRules: agentQueryRules,
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

func convertAgent(agentNode *bkGraphQL.ListJobsByQueueOrganizationJobsJobConnectionEdgesJobEdgeNodeJobTypeCommandAgent) buildkite.Agent {
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
	// FINISHED is the only state REST splits, by exit status. A job with no
	// exit status recorded counts as failed.
	if graphqlState == "FINISHED" {
		if exitStatus == "0" {
			return "passed"
		}
		return "failed"
	}

	return strings.ToLower(graphqlState)
}

// --state values with no single JobStates equivalent.
var inexactGraphQLStates = map[string][]bkGraphQL.JobStates{
	"passed": {bkGraphQL.JobStatesFinished},
	"failed": {bkGraphQL.JobStatesFinished},
}

// graphQLStatesFor maps --state values onto JobStates for the server-side
// filter. The bool reports whether the server alone produces the exact set;
// passed and failed are both FINISHED, so those still need the client pass.
func graphQLStatesFor(states []string) ([]bkGraphQL.JobStates, bool) {
	var mapped []bkGraphQL.JobStates
	seen := make(map[bkGraphQL.JobStates]struct{}, len(states))
	exact := true

	for _, state := range states {
		normalized := strings.ToLower(strings.TrimSpace(state))

		candidates, inexact := inexactGraphQLStates[normalized]
		if inexact {
			exact = false
		} else {
			// Uppercasing also covers states newer than the vendored schema.
			candidates = []bkGraphQL.JobStates{bkGraphQL.JobStates(strings.ToUpper(normalized))}
		}

		for _, candidate := range candidates {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			mapped = append(mapped, candidate)
		}
	}

	return mapped, exact
}

func jobListOptionsFromFlags(opts *jobListOptions) (*buildkite.BuildsListOptions, error) {
	if opts.build == "" && (opts.stepKey != "" || opts.groupKey != "") {
		return nil, fmt.Errorf("--step-key and --group-key require --build")
	}
	if opts.build != "" && (opts.since != "" || opts.until != "") {
		return nil, fmt.Errorf("--since and --until cannot be used with --build")
	}

	listOpts := &buildkite.BuildsListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: pageSize,
		},
		// Jobs are extracted from each build; the pipeline payload is unused,
		// so exclude it to keep responses small (important for large builds).
		ExcludePipeline: true,
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
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
	)

	pipeline, err := pipelineRes.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, org, pipeline.Name, listOpts)
	return builds, err
}

func applyClientSideFilters(jobs []buildkite.Job, opts jobListOptions, queueIDs []string) ([]buildkite.Job, error) {
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
			if !matchesQueue(*job, opts.queue, queueIDs) {
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

func matchesQueue(job buildkite.Job, queueFilter string, queueIDs []string) bool {
	if len(queueIDs) > 0 {
		return containsString(queueIDs, job.ClusterQueueID)
	}

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

func displayJobs(jobs []buildkite.Job, format output.Format, writer io.Writer) error {
	if format != output.FormatText {
		return output.Write(writer, jobs, format)
	}

	const (
		maxLabelLength  = 35
		truncatedLength = 32
		timeFormat      = "2006-01-02T15:04:05Z"
	)

	headers := []string{"State", "Label", "Started (UTC)", "Finished (UTC)", "Duration", "URL"}
	var rows [][]string

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

		rows = append(rows, []string{
			job.State,
			label,
			startedAt,
			finishedAt,
			duration,
			job.WebURL,
		})
	}

	table := output.Table(headers, rows, map[string]string{
		"state":          "bold",
		"label":          "italic",
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

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
