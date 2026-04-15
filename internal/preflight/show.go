package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	"github.com/buildkite/cli/v3/internal/build/watch"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

const showLimit = 100

type preflightRunsSummaryResponse struct {
	Tests preflightRunsSummaryTests `json:"tests"`
}

type preflightRunsSummaryTests struct {
	Runs     map[string]preflightRunSummary `json:"runs"`
	Failures []preflightRunFailure          `json:"failures"`
}

type preflightRunSummary struct {
	Suite   preflightRunSuite `json:"suite"`
	Passed  int               `json:"passed"`
	Failed  int               `json:"failed"`
	Skipped int               `json:"skipped"`
}

type preflightRunSuite struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

type preflightRunFailure struct {
	RunID         string                  `json:"run_id"`
	SuiteSlug     string                  `json:"suite_slug"`
	Name          string                  `json:"name"`
	Location      string                  `json:"location"`
	FailureReason string                  `json:"failure_reason"`
	LatestFail    *preflightRunLatestFail `json:"latest_fail,omitempty"`
}

type preflightRunLatestFail struct {
	FailureReason   string                      `json:"failure_reason"`
	FailureExpanded []buildkite.FailureExpanded `json:"failure_expanded,omitempty"`
}

type ShowResult struct {
	Status     string                   `json:"status"`
	DurationMS int64                    `json:"duration_ms"`
	BuildURL   string                   `json:"build_url"`
	Tests      map[string]ShowTestSuite `json:"tests"`
	Jobs       ShowJobs                 `json:"jobs"`
}

type ShowTestSuite struct {
	Passed   int               `json:"passed"`
	Failed   int               `json:"failed"`
	Skipped  int               `json:"skipped"`
	Failures []ShowTestFailure `json:"failures"`
}

type ShowTestFailure struct {
	Name          string              `json:"name"`
	Location      string              `json:"location"`
	Message       string              `json:"message"`
	FailureReason string              `json:"failure_reason"`
	FailureDetail []ShowFailureDetail `json:"failure_detail"`
}

type ShowFailureDetail struct {
	Backtrace []string `json:"backtrace"`
	Expanded  []string `json:"expanded"`
}

type ShowJobs struct {
	Passed     int             `json:"passed"`
	Failed     int             `json:"failed"`
	FailedJobs []ShowFailedJob `json:"failed_jobs"`
}

type ShowFailedJob struct {
	ID         string `json:"id"`
	ExitStatus *int   `json:"exit_status"`
	Name       string `json:"name"`
	Command    string `json:"command"`
	SoftFailed bool   `json:"soft_failed"`
	Retried    bool   `json:"retried"`
	State      string `json:"state"`
}

type ShowOptions struct {
	IncludeFailures bool
}

func LoadShowResult(ctx context.Context, client *buildkite.Client, org, preflightID string, opts ShowOptions) (ShowResult, error) {
	result := ShowResult{
		Tests: map[string]ShowTestSuite{},
		Jobs: ShowJobs{
			FailedJobs: []ShowFailedJob{},
		},
	}

	build, pipelineSlug, err := findPreflightBuild(ctx, client, org, preflightID)
	if err != nil {
		return ShowResult{}, err
	}

	build, _, err = client.Builds.Get(ctx, org, pipelineSlug, strconv.Itoa(build.Number), &buildkite.BuildGetOptions{
		BuildsListOptions: buildkite.BuildsListOptions{IncludeRetriedJobs: true},
		IncludeTestEngine: true,
	})
	if err != nil {
		return ShowResult{}, bkErrors.WrapAPIError(err, "loading preflight build")
	}

	result.Status = showBuildStatus(build)
	result.DurationMS = buildDurationMS(build)
	result.BuildURL = build.WebURL
	result.Jobs = summarizeJobs(build)

	tests, err := summarizeTests(ctx, client, org, build, opts)
	if err != nil {
		return ShowResult{}, err
	}
	result.Tests = tests

	return result, nil
}

func findPreflightBuild(ctx context.Context, client *buildkite.Client, org, preflightID string) (buildkite.Build, string, error) {
	builds, _, err := client.Builds.ListByOrg(ctx, org, &buildkite.BuildsListOptions{
		Branch:      []string{fmt.Sprintf("bk/preflight/%s", preflightID)},
		ExcludeJobs: true,
		ListOptions: buildkite.ListOptions{Page: 1, PerPage: 1},
	})
	if err != nil {
		return buildkite.Build{}, "", bkErrors.WrapAPIError(err, "finding preflight build")
	}
	if len(builds) == 0 {
		return buildkite.Build{}, "", bkErrors.NewResourceNotFoundError(
			nil,
			fmt.Sprintf("no preflight build found for %s", preflightID),
			"Verify the preflight UUID is correct",
			fmt.Sprintf("Ensure the preflight build was created in organization %q", org),
		)
	}

	pipelineSlug := pipelineSlugFromBuild(builds[0])
	if pipelineSlug == "" {
		return buildkite.Build{}, "", bkErrors.NewInternalError(
			nil,
			fmt.Sprintf("failed to determine pipeline slug for preflight build %s", preflightID),
			"This is likely a bug",
			"Report to Buildkite",
		)
	}

	return builds[0], pipelineSlug, nil
}

func summarizeJobs(build buildkite.Build) ShowJobs {
	tracker := watch.NewJobTracker()
	status := tracker.Update(build)

	summary := ShowJobs{
		Passed:     status.Summary.Passed,
		Failed:     status.Summary.Failed,
		FailedJobs: []ShowFailedJob{},
	}

	for _, job := range tracker.FailedJobs() {
		if len(summary.FailedJobs) >= showLimit {
			break
		}
		summary.FailedJobs = append(summary.FailedJobs, ShowFailedJob{
			ID:         job.ID,
			ExitStatus: job.ExitStatus,
			Name:       displayJobName(job),
			Command:    job.Command,
			SoftFailed: job.SoftFailed,
			Retried:    job.Retried,
			State:      job.State,
		})
	}

	return summary
}

func summarizeTests(ctx context.Context, client *buildkite.Client, org string, build buildkite.Build, opts ShowOptions) (map[string]ShowTestSuite, error) {
	suiteKey := primarySuiteSlug(build)
	if suiteKey == "" || build.ID == "" {
		return map[string]ShowTestSuite{}, nil
	}

	summary, err := loadPreflightRunsSummary(ctx, client, org, build.ID, opts)
	if err != nil {
		if isOptionalShowTestError(err) {
			return map[string]ShowTestSuite{}, nil
		}
		return nil, bkErrors.WrapAPIError(err, "loading preflight test summary")
	}

	tests := map[string]ShowTestSuite{}
	for _, run := range summary.Tests.Runs {
		slug := strings.TrimSpace(run.Suite.Slug)
		if slug == "" {
			slug = suiteKey
		}
		tests[slug] = ShowTestSuite{
			Passed:   run.Passed,
			Failed:   run.Failed,
			Skipped:  run.Skipped,
			Failures: []ShowTestFailure{},
		}
	}

	if !opts.IncludeFailures {
		return tests, nil
	}

	for _, failure := range summary.Tests.Failures {
		slug := strings.TrimSpace(failure.SuiteSlug)
		if slug == "" {
			slug = suiteKey
		}
		suite := tests[slug]
		if len(suite.Failures) >= showLimit {
			tests[slug] = suite
			continue
		}
		suite.Failures = append(suite.Failures, showRunFailure(failure))
		tests[slug] = suite
	}

	return tests, nil
}

func loadPreflightRunsSummary(ctx context.Context, client *buildkite.Client, org, buildID string, opts ShowOptions) (*preflightRunsSummaryResponse, error) {
	query := url.Values{}
	query.Set("build_id", buildID)
	query.Set("failed_result", "failed")
	if opts.IncludeFailures {
		query.Set("include", "latest_fail")
	}

	path := fmt.Sprintf("v2/organizations/%s/preflight/runs/%s?%s", org, buildID, query.Encode())
	req, err := client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var summary preflightRunsSummaryResponse
	_, err = client.Do(req, &summary)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

func showBuildStatus(build buildkite.Build) string {
	if build.Blocked {
		return string(buildstate.Blocked)
	}
	if build.State == "" {
		return "unknown"
	}
	return build.State
}

func buildDurationMS(build buildkite.Build) int64 {
	if build.FinishedAt == nil {
		return 0
	}

	start := build.StartedAt
	if start == nil {
		start = build.CreatedAt
	}
	if start == nil || build.FinishedAt.Before(start.Time) {
		return 0
	}

	return build.FinishedAt.Sub(start.Time).Milliseconds()
}

func primarySuiteSlug(build buildkite.Build) string {
	if build.TestEngine == nil {
		return ""
	}

	for _, run := range build.TestEngine.Runs {
		if slug := strings.TrimSpace(run.Suite.Slug); slug != "" {
			return slug
		}
	}

	return ""
}

func showRunFailure(runFailure preflightRunFailure) ShowTestFailure {
	failure := ShowTestFailure{
		Name:          strings.TrimSpace(runFailure.Name),
		Location:      runFailure.Location,
		FailureDetail: []ShowFailureDetail{},
	}

	if runFailure.LatestFail == nil {
		return failure
	}

	failure.Message = runFailure.LatestFail.FailureReason
	failure.FailureReason = runFailure.LatestFail.FailureReason
	for _, detail := range runFailure.LatestFail.FailureExpanded {
		failure.FailureDetail = append(failure.FailureDetail, ShowFailureDetail{
			Backtrace: detail.Backtrace,
			Expanded:  detail.Expanded,
		})
	}

	return failure
}

func displayJobName(job buildkite.Job) string {
	for _, candidate := range []string{job.Name, job.Label, job.StepKey} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func pipelineSlugFromBuild(build buildkite.Build) string {
	if build.Pipeline != nil && build.Pipeline.Slug != "" {
		return build.Pipeline.Slug
	}

	for _, rawURL := range []string{build.URL, build.WebURL} {
		if rawURL == "" {
			continue
		}

		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "pipelines" {
				return parts[i+1]
			}
		}
		if len(parts) >= 2 && parts[0] != "v2" {
			return parts[1]
		}
	}

	return ""
}

func isOptionalShowTestError(err error) bool {
	var apiErr *buildkite.ErrorResponse
	if !errors.As(err, &apiErr) || apiErr.Response == nil {
		return false
	}

	switch apiErr.Response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return false
	}
}
