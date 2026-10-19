package build

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type CompareCmd struct {
	BuildNumber string `arg:"" help:"Target build number, org/pipeline/number, or Buildkite build URL"`
	Baseline    *int   `help:"Baseline build number in the same pipeline (otherwise an earlier passed build on the same branch)."`
	Pipeline    string `help:"Pipeline slug or org/pipeline. Inferred from configuration or repository if omitted." short:"p"`
	output.OutputFlags
}

func (c *CompareCmd) Help() string {
	return `Compare final job attempts, not all retry attempts. Requires read_builds.
An explicit --baseline may be newer, failed or on another branch, but must be a
different positive build number in the same pipeline. Otherwise search up to 500
candidates for the latest earlier-created build that is currently passed on the
target branch. It need not have passed before the target started.

Match unique step keys with type, matrix and parallel index/total. Unkeyed jobs
use unique exact names within their group and coordinates (a heuristic, reported
as name_fallback). Duplicates and unnamed unkeyed jobs remain unmatched. Changes
in soft_failed are state_changed, not newly_failing/recovered.

All comparison rows are returned, failures first. Each inventory is limited to
10 pages (1000 jobs). Missing baselines, incomplete inventories and API failures
return an error without partial output. Unfinished builds are snapshots only.
Timings cover final attempts; positive execution deltas mean slower, not build
wall-clock changes. Still-failing does not establish a shared cause or retry
safety. No logs are fetched; use job IDs/URLs to investigate. JSON/YAML never
prompts for pipeline selection; use --pipeline when ambiguous.

Examples:
  $ bk build compare 123 --pipeline my-pipeline --json
  $ bk build compare acme/my-pipeline/123 --baseline 120 --text
  $ bk build compare https://buildkite.com/acme/my-pipeline/builds/123 --yaml
`
}

type comparisonBuild struct {
	ID        string               `json:"id" yaml:"id"`
	Number    int                  `json:"number" yaml:"number"`
	State     string               `json:"state" yaml:"state"`
	Branch    string               `json:"branch" yaml:"branch"`
	Commit    string               `json:"commit" yaml:"commit"`
	WebURL    string               `json:"web_url" yaml:"web_url"`
	CreatedAt *buildkite.Timestamp `json:"created_at" yaml:"created_at"`
}

func summarizeComparisonBuild(b buildkite.Build) comparisonBuild {
	return comparisonBuild{b.ID, b.Number, b.State, b.Branch, b.Commit, b.WebURL, b.CreatedAt}
}

type buildComparison struct {
	Organization      string           `json:"organization" yaml:"organization"`
	Pipeline          string           `json:"pipeline" yaml:"pipeline"`
	Target            comparisonBuild  `json:"target" yaml:"target"`
	Baseline          comparisonBuild  `json:"baseline" yaml:"baseline"`
	BaselineSelection string           `json:"baseline_selection" yaml:"baseline_selection"`
	CandidatesScanned int              `json:"candidates_scanned" yaml:"candidates_scanned"`
	TargetJobs        int              `json:"target_jobs" yaml:"target_jobs"`
	BaselineJobs      int              `json:"baseline_jobs" yaml:"baseline_jobs"`
	ChangeCounts      map[string]int   `json:"change_counts" yaml:"change_counts"`
	Steps             []stepComparison `json:"steps" yaml:"steps"`
	Warnings          []string         `json:"warnings" yaml:"warnings"`
}

func (r buildComparison) TextOutput() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s/%s: build %d vs %d (%s)\n", r.Organization, r.Pipeline, r.Target.Number, r.Baseline.Number, r.BaselineSelection)
	if len(r.Steps) == 0 {
		sb.WriteString("No final jobs in either build.\n")
	}
	for _, step := range r.Steps {
		fmt.Fprintf(&sb, "\n%s: key=%q type=%q group=%q\n", step.Change, step.StepKey, step.Type, step.GroupKey)
		if step.Matrix != nil {
			matrix, _ := json.Marshal(step.Matrix) // Already decoded from API JSON.
			fmt.Fprintf(&sb, "  Matrix: %s\n", matrix)
		}
		if step.ParallelIndex != nil {
			fmt.Fprintf(&sb, "  Parallel index: %d\n", *step.ParallelIndex)
		}
		if step.ParallelTotal != nil {
			fmt.Fprintf(&sb, "  Parallel total: %d\n", *step.ParallelTotal)
		}
		if step.MatchMethod != "" {
			fmt.Fprintf(&sb, "  Match: %s\n", step.MatchMethod)
		}
		if step.UnmatchedReason != "" {
			fmt.Fprintf(&sb, "  Unmatched: %s\n", step.UnmatchedReason)
		}
		for _, side := range []struct {
			label string
			job   *comparisonJob
		}{{"Baseline", step.Baseline}, {"Target", step.Target}} {
			if side.job == nil {
				continue
			}
			j := side.job
			fmt.Fprintf(&sb, "  %s: %s %q state=%s soft_failed=%t retries=%d\n", side.label, j.ID, j.Name, j.State, j.SoftFailed, j.RetriesCount)
			if j.ExecutionSeconds != nil {
				fmt.Fprintf(&sb, "    Execution: %.3fs\n", *j.ExecutionSeconds)
			}
			if j.SchedulingSeconds != nil {
				fmt.Fprintf(&sb, "    Scheduling: %.3fs\n", *j.SchedulingSeconds)
			}
			if j.WebURL != "" {
				fmt.Fprintf(&sb, "    %s\n", j.WebURL)
			}
		}
		if step.ExecutionDeltaSeconds != nil {
			fmt.Fprintf(&sb, "  Execution delta: %+.3fs\n", *step.ExecutionDeltaSeconds)
		}
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&sb, "\nNote: %s", warning)
	}
	return sb.String()
}

func (c *CompareCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	baselineNumber := 0
	if c.Baseline != nil {
		baselineNumber = *c.Baseline
		if baselineNumber <= 0 {
			return fmt.Errorf("--baseline must be a positive build number")
		}
	}
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	f.NoInput = f.NoInput || globals.DisableInput() || format != output.FormatText
	f.Quiet = f.Quiet || globals.IsQuiet()
	ctx := context.Background()
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)
	bld, err := buildResolver.ResolveFromPositionalArgument([]string{c.BuildNumber}, 0, pipelineRes.Resolve, f.Config)(ctx)
	if err != nil {
		return err
	}
	if bld.BuildNumber <= 0 || bld.BuildNumber == baselineNumber {
		return fmt.Errorf("target must be positive and different from --baseline")
	}
	if bld.Organization != f.Config.OrganizationSlug() {
		f, err = factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(bld.Organization))
		if err != nil {
			return err
		}
	}
	f.NoInput = f.NoInput || globals.DisableInput() || format != output.FormatText
	f.Quiet = f.Quiet || globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), bld.Organization); err != nil {
		return err
	}
	var result buildComparison
	if err := bkIO.SpinWhile(f, "Comparing builds", func() error {
		var err error
		result, err = loadBuildComparison(ctx, f.RestAPIClient, bld.Organization, bld.Pipeline, bld.BuildNumber, baselineNumber)
		return err
	}); err != nil {
		return fmt.Errorf("failed to compare builds (requires read_builds): %w", err)
	}
	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
		defer func() { _ = cleanup() }()
		return output.Write(writer, result, format)
	}
	return output.Write(kongCtx.Stdout, result, format)
}

func loadBuildComparison(ctx context.Context, client *buildkite.Client, org, pipeline string, number, baselineNumber int) (buildComparison, error) {
	r := buildComparison{Organization: org, Pipeline: pipeline, BaselineSelection: "explicit", ChangeCounts: map[string]int{}, Steps: []stepComparison{}, Warnings: []string{}}
	opts := &buildkite.BuildGetOptions{BuildsListOptions: buildkite.BuildsListOptions{ExcludeJobs: true, ExcludePipeline: true}}
	target, _, err := client.Builds.Get(ctx, org, pipeline, strconv.Itoa(number), opts)
	if err != nil {
		return r, err
	}
	r.Target = summarizeComparisonBuild(target)
	if baselineNumber == 0 {
		r.BaselineSelection = "most_recently_created_earlier_passed_build_on_same_branch"
		baselineNumber, r.CandidatesScanned, err = findComparisonBaseline(ctx, client, org, pipeline, target)
		if err != nil {
			return r, err
		}
	}
	baseline, _, err := client.Builds.Get(ctx, org, pipeline, strconv.Itoa(baselineNumber), opts)
	if err != nil {
		return r, err
	}
	r.Baseline = summarizeComparisonBuild(baseline)
	if target.Branch != baseline.Branch {
		r.Warnings = append(r.Warnings, "Explicit baseline is on a different branch.")
	}
	if target.FinishedAt == nil || baseline.FinishedAt == nil {
		r.Warnings = append(r.Warnings, "One or both builds are unfinished; jobs can change or be uploaded after this snapshot. Added/removed means present/absent in this snapshot only.")
	}
	targetJobs, err := loadComparisonJobs(ctx, client, org, pipeline, number)
	if err != nil {
		return r, err
	}
	baselineJobs, err := loadComparisonJobs(ctx, client, org, pipeline, baselineNumber)
	if err != nil {
		return r, err
	}
	r.TargetJobs, r.BaselineJobs = len(targetJobs), len(baselineJobs)
	compareBuildJobs(targetJobs, baselineJobs, &r)
	r.Warnings = append(r.Warnings, "Timings cover final attempts only; scheduling_seconds is scheduled_at to started_at and can include upstream waiting when uploaded jobs inherit the build's creation time. It is not isolated scheduler/agent delay. Execution deltas are not build wall-clock deltas. Still-failing does not prove a shared cause or that retrying is safe. No logs were fetched; inspect job IDs/URLs for evidence.")
	return r, nil
}

func findComparisonBaseline(ctx context.Context, client *buildkite.Client, org, pipeline string, target buildkite.Build) (int, int, error) {
	if target.Branch == "" || target.CreatedAt == nil {
		return 0, 0, fmt.Errorf("target has no branch or creation time; provide --baseline")
	}
	// The SDK encodes whole seconds. Include same-second candidates here, then
	// enforce the precise, strictly earlier timestamp below.
	cutoff := target.CreatedAt.Truncate(time.Second).Add(time.Second)
	opts := &buildkite.BuildsListOptions{Branch: []string{target.Branch}, State: []string{"passed"}, CreatedTo: cutoff, ExcludeJobs: true, ExcludePipeline: true, ListOptions: buildkite.ListOptions{Page: 1, PerPage: 100}}
	scanned := 0
	for range 5 {
		builds, resp, err := client.Builds.ListByPipeline(ctx, org, pipeline, opts)
		if err != nil {
			return 0, scanned, err
		}
		for _, b := range builds {
			scanned++
			if b.Number < target.Number && b.State == "passed" && b.Branch == target.Branch && b.CreatedAt != nil && b.CreatedAt.Before(target.CreatedAt.Time) {
				return b.Number, scanned, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return 0, scanned, fmt.Errorf("no earlier successful same-branch baseline found within 5 pages (500 candidates); provide --baseline")
}

func loadComparisonJobs(ctx context.Context, client *buildkite.Client, org, pipeline string, number int) ([]buildkite.Job, error) {
	includeRetried := false
	opts := &buildkite.JobsListOptions{PerPage: 100, IncludeRetriedJobs: &includeRetried}
	jobs := []buildkite.Job{}
	for range 10 {
		page, _, err := client.Jobs.ListByBuild(ctx, org, pipeline, strconv.Itoa(number), opts)
		if err != nil {
			return nil, err
		}
		for _, job := range page.Items {
			if !job.Retried {
				jobs = append(jobs, job)
			}
		}
		if page.Links.Next == "" {
			return jobs, nil
		}
		opts, err = page.Links.Next.ToOptions()
		if err != nil {
			return nil, fmt.Errorf("invalid next jobs page: %w", err)
		}
		opts.PerPage, opts.IncludeRetriedJobs = 100, &includeRetried
	}
	return nil, fmt.Errorf("build %d exceeds 10 pages (1000 jobs); no partial comparison produced; use bk job list --help to narrow the inventory", number)
}
