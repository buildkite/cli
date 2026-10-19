package build

import (
	"context"
	"fmt"
	"strings"

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

type FailedTestsCmd struct {
	BuildNumber string `arg:"" help:"Build number, org/pipeline/number, or Buildkite build URL"`
	Pipeline    string `help:"Pipeline slug or org/pipeline. Inferred from configuration or repository if omitted." short:"p"`
	output.OutputFlags
}

func (c *FailedTestsCmd) Help() string {
	return `List tests with at least one failed execution in this build, including tests
that passed on retry, regardless of their current flaky labels or the build state.
All result pages are fetched. Requires read_builds and read_suites token scopes.
JSON/YAML never prompts for pipeline selection; use --pipeline when ambiguous.

Results include test details, current labels, and build execution metrics, not
individual failure messages or backtraces. Labels are not a per-failure verdict.
Only uploaded Test Engine data from suites accessible to your token is included;
an empty result does not prove that no tests failed. The API's aggregation window
is limited by retention and ends at most 24 hours after build creation.

Examples:
  $ bk build failed-tests 123 --pipeline my-pipeline --json
  $ bk build failed-tests acme/my-pipeline/123 --text
  $ bk build failed-tests https://buildkite.com/acme/my-pipeline/builds/123
`
}

const failedTestsNote = "Only available Test Engine data from accessible suites is included. No matches does not prove that no tests failed. Current labels are not a per-failure diagnosis."

type failedTestsOutput struct {
	Organization string                      `json:"organization" yaml:"organization"`
	Pipeline     string                      `json:"pipeline" yaml:"pipeline"`
	BuildNumber  int                         `json:"build_number" yaml:"build_number"`
	BuildID      string                      `json:"build_id" yaml:"build_id"`
	Tests        []buildkite.TestWithMetrics `json:"failed_tests" yaml:"failed_tests"`
	Note         string                      `json:"note" yaml:"note"`
}

func (r failedTestsOutput) TextOutput() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Tests with failed executions for %s/%s/%d (%d)\n\n", r.Organization, r.Pipeline, r.BuildNumber, len(r.Tests))
	if len(r.Tests) == 0 {
		sb.WriteString("No matching tests found.\n")
	} else {
		rows := make([][]string, 0, len(r.Tests))
		for _, test := range r.Tests {
			rows = append(rows, []string{test.ID, test.Scope, test.Name, test.Location, test.WebURL})
		}
		sb.WriteString(output.Table([]string{"ID", "Scope", "Name", "Location", "URL"}, rows, nil))
	}
	fmt.Fprintf(&sb, "\n%s", r.Note)
	return sb.String()
}

func (c *FailedTestsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	// Pipeline prompts write to stdout, so structured output must not prompt.
	f.NoInput = globals.DisableInput() || format != output.FormatText
	f.Quiet = globals.IsQuiet()
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
	if bld.Organization != f.Config.OrganizationSlug() {
		f, err = factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(bld.Organization))
		if err != nil {
			return err
		}
	}
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), bld.Organization); err != nil {
		return err
	}

	result := failedTestsOutput{
		Organization: bld.Organization,
		Pipeline:     bld.Pipeline,
		BuildNumber:  bld.BuildNumber,
		Tests:        []buildkite.TestWithMetrics{},
		Note:         failedTestsNote,
	}
	if err := bkIO.SpinWhile(f, "Loading failed tests", func() error {
		build, _, err := f.RestAPIClient.Builds.Get(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.BuildGetOptions{
			BuildsListOptions: buildkite.BuildsListOptions{ExcludeJobs: true, ExcludePipeline: true},
		})
		if err != nil {
			return fmt.Errorf("failed to get build (requires read_builds): %w", err)
		}
		if build.ID == "" {
			return fmt.Errorf("build has no UUID")
		}
		result.BuildID = build.ID
		opts := &buildkite.BuildTestsListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: pageSize},
			Tags:        "result:~failed",
		}
		for {
			tests, resp, err := f.RestAPIClient.BuildTests.List(ctx, bld.Organization, build.ID, opts)
			if err != nil {
				return fmt.Errorf("failed to list Test Engine tests (requires read_suites): %w", err)
			}
			result.Tests = append(result.Tests, tests...)
			if resp.NextPage == 0 {
				return nil
			}
			opts.Page = resp.NextPage
		}
	}); err != nil {
		return err
	}

	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
		defer func() { _ = cleanup() }()
		return output.Write(writer, result, format)
	}
	return output.Write(kongCtx.Stdout, result, format)
}
