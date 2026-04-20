package preflight

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type SummaryOptions struct {
	IncludeFailures bool
}

type SummaryResult struct {
	Tests    map[string]SummaryTestRun `json:"tests"`
	Failures []SummaryTestFailure      `json:"failures"`
}

type SummaryTestRun struct {
	RunID     string `json:"run_id"`
	SuiteName string `json:"suite_name,omitempty"`
	SuiteSlug string `json:"suite_slug"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
}

type SummaryTestFailure struct {
	RunID         string                 `json:"run_id"`
	SuiteName     string                 `json:"suite_name,omitempty"`
	SuiteSlug     string                 `json:"suite_slug"`
	Name          string                 `json:"name"`
	Location      string                 `json:"location"`
	Message       string                 `json:"message"`
	FailureReason string                 `json:"failure_reason"`
	FailureDetail []SummaryFailureDetail `json:"failure_detail"`
}

type SummaryFailureDetail struct {
	Backtrace []string `json:"backtrace"`
	Expanded  []string `json:"expanded"`
}

type RunSummaryService struct {
	client *buildkite.Client
}

type RunSummaryGetOptions struct {
	Result    string
	IncludeFailures bool
}

type RunSummaryResponse struct {
	Tests RunSummaryTests `json:"tests"`
}

type RunSummaryTests struct {
	Runs     map[string]RunSummaryRun `json:"runs"`
	Failures []RunSummaryFailure      `json:"failures"`
}

type RunSummaryRun struct {
	Suite   RunSummarySuite `json:"suite"`
	Passed  int             `json:"passed"`
	Failed  int             `json:"failed"`
	Skipped int             `json:"skipped"`
}

type RunSummarySuite struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type RunSummaryFailure struct {
	RunID         string                `json:"run_id"`
	SuiteName     string                `json:"suite_name"`
	SuiteSlug     string                `json:"suite_slug"`
	Name          string                `json:"name"`
	Location      string                `json:"location"`
	FailureReason string                `json:"failure_reason"`
	LatestFail    *RunSummaryLatestFail `json:"latest_fail,omitempty"`
}

type RunSummaryLatestFail struct {
	FailureReason   string                      `json:"failure_reason"`
	FailureExpanded []buildkite.FailureExpanded `json:"failure_expanded,omitempty"`
}

func NewRunSummaryService(client *buildkite.Client) *RunSummaryService {
	return &RunSummaryService{client: client}
}

func (s *RunSummaryService) Get(ctx context.Context, org, buildID string, opt *RunSummaryGetOptions) (*RunSummaryResponse, error) {
	query := url.Values{}
	if opt != nil {
		if opt.Result != "" {
			query.Set("result", opt.Result)
		}
		if opt.IncludeFailures {
			query.Set("include", "latest_fail")
		}
	}

	u := fmt.Sprintf("v2/analytics/organizations/%s/builds/%s/preflight/v1", org, buildID)
	if encoded := query.Encode(); encoded != "" {
		u += "?" + encoded
	}

	req, err := s.client.NewRequest(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	var summary RunSummaryResponse
	_, err = s.client.Do(req, &summary)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

func (r RunSummaryResponse) SummaryResult() SummaryResult {
	tests := make(map[string]SummaryTestRun, len(r.Tests.Runs))

	for runID, run := range r.Tests.Runs {
		tests[runID] = SummaryTestRun{
			RunID:     runID,
			SuiteName: strings.TrimSpace(run.Suite.Name),
			SuiteSlug: strings.TrimSpace(run.Suite.Slug),
			Passed:    run.Passed,
			Failed:    run.Failed,
			Skipped:   run.Skipped,
		}
	}

	failures := make([]SummaryTestFailure, 0, len(r.Tests.Failures))
	for _, failure := range r.Tests.Failures {
		failures = append(failures, failure.summaryFailure())
	}

	return SummaryResult{Tests: SummaryTests{Runs: tests, Failures: failures}}
}

func (f RunSummaryFailure) summaryFailure() SummaryTestFailure {
	result := SummaryTestFailure{
		RunID:         strings.TrimSpace(f.RunID),
		SuiteName:     strings.TrimSpace(f.SuiteName),
		SuiteSlug:     strings.TrimSpace(f.SuiteSlug),
		Name:          strings.TrimSpace(f.Name),
		Location:      f.Location,
		FailureReason: f.FailureReason,
		FailureDetail: []SummaryFailureDetail{},
	}

	if f.LatestFail == nil {
		result.Message = f.FailureReason
		return result
	}

	result.Message = f.LatestFail.FailureReason
	if result.FailureReason == "" {
		result.FailureReason = f.LatestFail.FailureReason
	}

	for _, detail := range f.LatestFail.FailureExpanded {
		result.FailureDetail = append(result.FailureDetail, SummaryFailureDetail{
			Backtrace: detail.Backtrace,
			Expanded:  detail.Expanded,
		})
	}

	return result
}
