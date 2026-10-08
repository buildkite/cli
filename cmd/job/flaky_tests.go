package job

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/google/uuid"
)

type FlakyTestsCmd struct {
	JobID string `arg:"" help:"Failed job UUID to inspect"`
	output.OutputFlags
}

func (c *FlakyTestsCmd) Help() string {
	return `List tests with failed executions for this job that Test Engine currently labels flaky.
All result pages are fetched. Requires read_builds and read_suites token scopes.
Executions must have the build.job_id tag matching the job UUID. Collectors normally
derive this from BUILDKITE_JOB_ID; overridden or missing job metadata may prevent matches.

These are current test labels, not a verdict that flakiness caused the job to fail.
No matches does not mean not flaky: Test Engine data may be missing or incomplete.

Examples:
  $ bk job flaky-tests 0190046e-e199-453b-a302-a21a4d649d31 --text
  $ bk job flaky-tests 0190046e-e199-453b-a302-a21a4d649d31 --json
`
}

const flakyTestsNote = "Current flaky test labels do not establish the cause of the job failure. No matches does not mean not flaky; Test Engine data may be missing or incomplete."

type flakyTestsOutput struct {
	Organization string                      `json:"organization" yaml:"organization"`
	BuildID      string                      `json:"build_id" yaml:"build_id"`
	JobID        string                      `json:"job_id" yaml:"job_id"`
	Tests        []buildkite.TestWithMetrics `json:"flaky_tests" yaml:"flaky_tests"`
	Note         string                      `json:"note" yaml:"note"`
}

func (r flakyTestsOutput) TextOutput() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Failed tests currently labeled flaky by Test Engine for job %s (%d)\n\n", r.JobID, len(r.Tests))
	if len(r.Tests) == 0 {
		sb.WriteString("No matching tests found.\n")
	} else {
		rows := make([][]string, 0, len(r.Tests))
		for _, test := range r.Tests {
			rows = append(rows, []string{test.ID, test.Scope, test.Name, test.WebURL})
		}
		sb.WriteString(output.Table([]string{"ID", "Scope", "Name", "URL"}, rows, nil))
	}
	fmt.Fprintf(&sb, "\n%s", r.Note)
	return sb.String()
}

func (c *FlakyTestsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	org, err := configuredOrganization(f.Config.OrganizationSlug())
	if err != nil {
		return err
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	var result flakyTestsOutput
	if err := bkIO.SpinWhile(f, "Loading flaky tests", func() error {
		result, err = fetchJobFlakyTests(context.Background(), f.RestAPIClient, org, c.JobID)
		return err
	}); err != nil {
		return err
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
		defer func() { _ = cleanup() }()
		return output.Write(writer, result, format)
	}
	return output.Write(kongCtx.Stdout, result, format)
}

func fetchJobFlakyTests(ctx context.Context, client *buildkite.Client, org, jobID string) (flakyTestsOutput, error) {
	result := flakyTestsOutput{Organization: org, Tests: []buildkite.TestWithMetrics{}, Note: flakyTestsNote}
	parsedID, err := uuid.Parse(jobID)
	if err != nil {
		return result, fmt.Errorf("invalid job UUID: %w", err)
	}
	result.JobID = parsedID.String()
	job, _, err := client.Jobs.GetJobByOrg(ctx, org, result.JobID)
	if err != nil {
		return result, fmt.Errorf("failed to get job: %w", err)
	}
	if job.State != "failed" {
		return result, fmt.Errorf("job %s is %q, not failed", result.JobID, job.State)
	}

	// Use the API build path to resolve the UUID without following an arbitrary
	// host or sending credentials outside the configured API endpoint.
	buildURL, err := url.Parse(job.BuildURL)
	if err != nil {
		return result, fmt.Errorf("invalid job build URL: %w", err)
	}
	parts := strings.Split(strings.Trim(buildURL.Path, "/"), "/")
	if len(parts) != 7 || parts[0] != "v2" || parts[1] != "organizations" || parts[2] != org || parts[3] != "pipelines" || parts[5] != "builds" {
		return result, fmt.Errorf("cannot resolve build from job build URL %q", job.BuildURL)
	}
	build, _, err := client.Builds.Get(ctx, org, parts[4], parts[6], &buildkite.BuildGetOptions{
		BuildsListOptions: buildkite.BuildsListOptions{ExcludeJobs: true, ExcludePipeline: true},
	})
	if err != nil {
		return result, fmt.Errorf("failed to get job's build: %w", err)
	}
	if build.ID == "" {
		return result, fmt.Errorf("job's build has no UUID")
	}
	result.BuildID = build.ID

	opts := &buildkite.BuildTestsListOptions{
		ListOptions: buildkite.ListOptions{Page: 1, PerPage: pageSize},
		Labels:      "flaky",
		Tags:        "build.job_id:" + result.JobID + ",result:~failed",
	}
	for {
		tests, resp, err := client.BuildTests.List(ctx, org, build.ID, opts)
		if err != nil {
			return result, fmt.Errorf("failed to list Test Engine tests (requires read_suites): %w", err)
		}
		result.Tests = append(result.Tests, tests...)
		if resp.NextPage == 0 {
			return result, nil
		}
		opts.Page = resp.NextPage
	}
}
