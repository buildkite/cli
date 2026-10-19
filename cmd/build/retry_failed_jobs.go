package build

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
)

type RetryFailedJobsCmd struct {
	BuildNumber string `arg:"" help:"Build number, org/pipeline/number, or Buildkite build URL"`
	Pipeline    string `help:"Pipeline slug or org/pipeline. Inferred from configuration or repository if omitted." short:"p"`
	States      string `help:"Comma-separated failure states: canceled,expired,failed,soft_failed,timed_out. Omitted or empty retries all retryable failed jobs."`
	output.OutputFlags
}

func (c *RetryFailedJobsCmd) Help() string {
	return `Queue retries for failed jobs in an existing build. Requires write_builds.
By default, retries all retryable failed jobs. Use --states to select failure
types. This is not limited to infrastructure failures and does not check whether
jobs are safe to rerun; retries can repeat side effects, including deployments.

Requires confirmation unless --yes is supplied. --no-input without --yes fails
without retrying. JSON/YAML never prompts for pipeline selection; use --pipeline
when ambiguous. Confirmation goes to stderr, not structured stdout.
Malformed or conflicting explicit build/pipeline selectors are rejected.

The API returns 202 Accepted asynchronously. retried_jobs_count is the number of
jobs queued for retry, not completed or successful retries. No waiting is done;
use bk build view or bk build watch to follow the build.
If the accepted response has no usable count, check the build before resubmitting.

Examples:
  $ bk build retry-failed-jobs 123 --pipeline my-pipeline
  $ bk build retry-failed-jobs acme/my-pipeline/123 --states failed,soft_failed --yes --json
  $ bk build retry-failed-jobs https://buildkite.com/acme/my-pipeline/builds/123 --yes --no-input --text
`
}

type retryFailedJobsOutput struct {
	Organization     string `json:"organization" yaml:"organization"`
	Pipeline         string `json:"pipeline" yaml:"pipeline"`
	BuildNumber      int    `json:"build_number" yaml:"build_number"`
	RetriedJobsCount int    `json:"retried_jobs_count" yaml:"retried_jobs_count"`
	WebURL           string `json:"web_url" yaml:"web_url"`
	Note             string `json:"note" yaml:"note"`
}

func (c *RetryFailedJobsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	// Shared read selectors tolerate extra path components. A bulk mutation must
	// reject them rather than silently reinterpret the user's target.
	const slug = `[A-Za-z0-9][A-Za-z0-9_-]*`
	if ok, _ := regexp.MatchString(`^(?:[0-9]+|`+slug+`/`+slug+`/[0-9]+|https://buildkite\.com/`+slug+`/`+slug+`/builds/[0-9]+)$`, c.BuildNumber); !ok {
		return fmt.Errorf("unable to parse build: use a positive number, org/pipeline/number, or https://buildkite.com/org/pipeline/builds/number")
	}
	if c.Pipeline != "" {
		if ok, _ := regexp.MatchString(`^(?:`+slug+`/)?`+slug+`$`, c.Pipeline); !ok {
			return fmt.Errorf("invalid --pipeline: use a pipeline slug or org/pipeline")
		}
	}
	if c.States != "" {
		for _, state := range strings.Split(c.States, ",") {
			switch state {
			case "canceled", "expired", "failed", "soft_failed", "timed_out":
			default:
				return fmt.Errorf("invalid state %q: use canceled,expired,failed,soft_failed,timed_out", state)
			}
		}
	}
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	switch format {
	case output.FormatJSON, output.FormatYAML, output.FormatText:
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	noInput := f.NoInput || globals.DisableInput()
	// Selection prompts write to stdout; never mix them into JSON/YAML.
	f.NoInput = noInput || format != output.FormatText
	f.Quiet = f.Quiet || globals.IsQuiet()
	ctx := context.Background()
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)
	// Match pipeline/root target normalization before selecting credentials.
	buildArg := strings.ReplaceAll(c.BuildNumber, "_", "-")
	bld, err := buildResolver.ResolveFromPositionalArgument([]string{buildArg}, 0, pipelineRes.Resolve, f.Config)(ctx)
	if err != nil {
		return err
	}
	if bld.BuildNumber <= 0 {
		return fmt.Errorf("build number must be positive")
	}
	if c.Pipeline != "" {
		explicit := strings.Split(strings.ReplaceAll(c.Pipeline, "_", "-"), "/")
		if explicit[len(explicit)-1] != bld.Pipeline || (len(explicit) == 2 && explicit[0] != bld.Organization) {
			return fmt.Errorf("--pipeline %q conflicts with build %s/%s/%d", c.Pipeline, bld.Organization, bld.Pipeline, bld.BuildNumber)
		}
	}
	if bld.Organization != f.Config.OrganizationSlug() {
		f, err = factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(bld.Organization))
		if err != nil {
			return err
		}
	}
	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = noInput
	f.Quiet = f.Quiet || globals.IsQuiet()
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), bld.Organization); err != nil {
		return err
	}
	failureTypes := c.States
	if failureTypes == "" {
		failureTypes = "all retryable failure types"
	}
	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Queue retries for %s/%s/%d (%s)? Not limited to infrastructure failures; may repeat side effects, including deployments", bld.Organization, bld.Pipeline, bld.BuildNumber, failureTypes))
	if err != nil || !confirmed {
		return err
	}

	path := fmt.Sprintf("v2/organizations/%s/pipelines/%s/builds/%d/retry_failed_jobs", url.PathEscape(bld.Organization), url.PathEscape(bld.Pipeline), bld.BuildNumber)
	req, err := f.RestAPIClient.NewRequest(ctx, http.MethodPut, path, struct {
		States string `json:"states,omitempty"`
	}{States: c.States})
	if err != nil {
		return err
	}
	// Decode only API-owned fields, not the locally constructed receipt. The
	// build response includes a pipeline object, not our pipeline slug string.
	var response struct {
		RetriedJobsCount *int   `json:"retried_jobs_count"`
		WebURL           string `json:"web_url"`
	}
	resp, err := f.RestAPIClient.Do(req, &response)
	if resp != nil && resp.StatusCode == http.StatusAccepted {
		if err != nil || response.RetriedJobsCount == nil || *response.RetriedJobsCount < 0 {
			return fmt.Errorf("retry request accepted, but its queued-job receipt is unavailable; check the build before resubmitting")
		}
	} else {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected retry response (expected 202 Accepted); check the build before resubmitting")
	}
	result := retryFailedJobsOutput{
		Organization:     bld.Organization,
		Pipeline:         bld.Pipeline,
		BuildNumber:      bld.BuildNumber,
		RetriedJobsCount: *response.RetriedJobsCount,
		WebURL:           response.WebURL,
		Note:             "Accepted asynchronously. retried_jobs_count counts jobs queued for retry, not completed or successful retries.",
	}
	if err := output.WriteTextOrStructured(kongCtx.Stdout, format, result,
		fmt.Sprintf("Retry request accepted for %s/%s/%d: %d jobs queued for retry (not completed). %s", bld.Organization, bld.Pipeline, bld.BuildNumber, result.RetriedJobsCount, result.WebURL)); err != nil {
		return fmt.Errorf("retry request accepted, but writing its receipt failed; check the build before resubmitting: %w", err)
	}
	return nil
}
