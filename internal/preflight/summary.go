package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

const summaryLimit = 100

func LoadSummaryResult(ctx context.Context, client *buildkite.Client, org, preflightID string, opts SummaryOptions) (SummaryResult, error) {
	result := SummaryResult{
		Tests:    map[string]SummaryTestRun{},
		Failures: []SummaryTestFailure{},
	}

	build, pipelineSlug, err := findPreflightBuild(ctx, client, org, preflightID)
	if err != nil {
		return SummaryResult{}, err
	}

	build, _, err = client.Builds.Get(ctx, org, pipelineSlug, strconv.Itoa(build.Number), &buildkite.BuildGetOptions{
		BuildsListOptions: buildkite.BuildsListOptions{IncludeRetriedJobs: true},
		IncludeTestEngine: true,
	})
	if err != nil {
		return SummaryResult{}, bkErrors.WrapAPIError(err, "loading preflight build")
	}

	summary, err := NewRunSummaryService(client).Get(ctx, org, build.ID, &RunSummaryGetOptions{
		FailedResult:    "^failed",
		IncludeFailures: opts.IncludeFailures,
	})

	if err != nil {
		if isOptionalSummaryTestError(err) {
			return result, nil
		}
		return SummaryResult{}, bkErrors.WrapAPIError(err, "loading preflight test summary")
	}

	result = summary.SummaryResult()

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

	pipelineSlug := ""
	if builds[0].Pipeline != nil {
		pipelineSlug = builds[0].Pipeline.Slug
	}
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

func isOptionalSummaryTestError(err error) bool {
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
