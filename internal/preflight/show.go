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

func LoadShowResult(ctx context.Context, client *buildkite.Client, org, preflightID string) (ShowResult, error) {
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

	tests, err := summarizeTests(ctx, client, org, build)
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

func summarizeTests(ctx context.Context, client *buildkite.Client, org string, build buildkite.Build) (map[string]ShowTestSuite, error) {
	suiteKey := primarySuiteSlug(build)
	if suiteKey == "" || build.ID == "" {
		return map[string]ShowTestSuite{}, nil
	}

	allTests, err := listBuildTests(ctx, client, org, build.ID, buildkite.BuildTestsListOptions{State: "enabled"}, 0)
	if err != nil {
		if isOptionalShowTestError(err) {
			return map[string]ShowTestSuite{}, nil
		}
		return nil, bkErrors.WrapAPIError(err, "loading preflight test summary")
	}

	failedTests, err := listBuildTests(ctx, client, org, build.ID, buildkite.BuildTestsListOptions{
		Result:  "failed",
		State:   "enabled",
		Include: "latest_fail",
	}, showLimit)
	if err != nil {
		if isOptionalShowTestError(err) {
			return map[string]ShowTestSuite{}, nil
		}
		return nil, bkErrors.WrapAPIError(err, "loading preflight test failures")
	}

	// The build tests API is scoped to a build rather than a suite, so we can
	// only produce one aggregated summary for the build today.
	suite := ShowTestSuite{Failures: []ShowTestFailure{}}
	for _, test := range allTests {
		suite.Passed += test.ExecutionsCountByResult.Passed
		suite.Failed += test.ExecutionsCountByResult.Failed
		suite.Skipped += test.ExecutionsCountByResult.Skipped
	}
	for _, test := range failedTests {
		suite.Failures = append(suite.Failures, showTestFailure(test))
	}

	return map[string]ShowTestSuite{suiteKey: suite}, nil
}

func listBuildTests(ctx context.Context, client *buildkite.Client, org, buildID string, opt buildkite.BuildTestsListOptions, limit int) ([]buildkite.BuildTest, error) {
	opt.ListOptions = buildkite.ListOptions{Page: 1, PerPage: 100}
	var tests []buildkite.BuildTest

	for {
		pageTests, resp, err := client.BuildTests.List(ctx, org, buildID, &opt)
		if err != nil {
			return nil, err
		}

		if limit > 0 && len(tests)+len(pageTests) > limit {
			pageTests = pageTests[:limit-len(tests)]
		}
		tests = append(tests, pageTests...)

		if limit > 0 && len(tests) >= limit {
			return tests, nil
		}
		if resp == nil || resp.NextPage == 0 {
			return tests, nil
		}

		opt.Page = resp.NextPage
	}
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

func showTestFailure(test buildkite.BuildTest) ShowTestFailure {
	failure := ShowTestFailure{
		Name:          displayTestName(test),
		Location:      test.Location,
		FailureDetail: []ShowFailureDetail{},
	}

	if test.LatestFail == nil {
		return failure
	}

	failure.Message = test.LatestFail.FailureReason
	failure.FailureReason = test.LatestFail.FailureReason
	for _, detail := range test.LatestFail.FailureExpanded {
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

func displayTestName(test buildkite.BuildTest) string {
	scope := strings.TrimSpace(test.Scope)
	name := strings.TrimSpace(test.Name)
	switch {
	case scope == "":
		return name
	case name == "":
		return scope
	default:
		return scope + "." + name
	}
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
