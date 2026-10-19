package build

import (
	"context"
	"fmt"
	"net/http"
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

type StepUploadsCmd struct {
	BuildNumber string `arg:"" help:"Build number, org/pipeline/number, or Buildkite build URL"`
	Pipeline    string `help:"Pipeline slug or org/pipeline. Inferred from configuration or repository if omitted." short:"p"`
	output.OutputFlags
}

func (c *StepUploadsCmd) Help() string {
	return `List a build's dynamic step uploads, newest first, fetching all cursor pages.
Requires the read_builds token scope. JSON/YAML never prompts for pipeline
selection; use --pipeline when ambiguous.

Includes state, source job, created job count, rejection type and outcome message.
An unavailable count is omitted in JSON and shown as "-" in text, not zero.
Messages do not include uploaded configuration or internal failure details;
check the source job's pipeline upload command output for further diagnostics.
Uploaded definitions are not included. History is only available within the
build's maximum lifetime (up to 30 days after creation); older builds return 410.

Examples:
  $ bk build step-uploads 123 --pipeline my-pipeline --json
  $ bk build step-uploads acme/my-pipeline/123 --text
  $ bk build step-uploads https://buildkite.com/acme/my-pipeline/builds/123 --yaml
`
}

const stepUploadsNote = "History is limited to the build's maximum lifetime (up to 30 days after creation). For further rejection/failure diagnostics, check the source job's pipeline upload command output."

type stepUploadsOutput struct {
	Organization string                 `json:"organization" yaml:"organization"`
	Pipeline     string                 `json:"pipeline" yaml:"pipeline"`
	BuildNumber  int                    `json:"build_number" yaml:"build_number"`
	StepUploads  []buildkite.StepUpload `json:"step_uploads" yaml:"step_uploads"`
	Note         string                 `json:"note" yaml:"note"`
}

func (r stepUploadsOutput) TextOutput() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Step uploads for %s/%s/%d (%d)\n\n", r.Organization, r.Pipeline, r.BuildNumber, len(r.StepUploads))
	if len(r.StepUploads) == 0 {
		sb.WriteString("No step uploads found.\n")
	}
	for _, upload := range r.StepUploads {
		count := "-"
		if upload.CreatedJobsCount != nil {
			count = fmt.Sprint(*upload.CreatedJobsCount)
		}
		fmt.Fprintf(&sb, "%s (%s)\n  Source: %s  Job: %s\n  Created jobs: %s  Replace existing steps: %t\n", upload.UUID, upload.State, upload.Source, upload.SourceJobID, count, upload.ReplaceExistingSteps)
		if upload.RejectionType != nil {
			fmt.Fprintf(&sb, "  Rejection: %s\n", *upload.RejectionType)
		}
		if upload.Message != nil {
			fmt.Fprintf(&sb, "  Message: %s\n", *upload.Message)
		}
		sb.WriteString("\n")
	}
	sb.WriteString(r.Note)
	return sb.String()
}

func (c *StepUploadsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	// Pipeline prompts write to stdout, so structured output must not prompt.
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

	result := stepUploadsOutput{
		Organization: bld.Organization,
		Pipeline:     bld.Pipeline,
		BuildNumber:  bld.BuildNumber,
		StepUploads:  []buildkite.StepUpload{},
		Note:         stepUploadsNote,
	}
	if err := bkIO.SpinWhile(f, "Loading step uploads", func() error {
		opts := &buildkite.StepUploadsListOptions{PerPage: pageSize}
		for {
			page, resp, err := f.RestAPIClient.StepUploads.ListByBuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), opts)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusGone {
					return fmt.Errorf("step upload history is unavailable outside the build's maximum lifetime (up to 30 days after creation): %w", err)
				}
				return fmt.Errorf("failed to list step uploads (requires read_builds): %w", err)
			}
			result.StepUploads = append(result.StepUploads, page.Items...)
			if page.Links.Next == "" {
				return nil
			}
			opts, err = page.Links.Next.ToOptions()
			if err != nil {
				return fmt.Errorf("invalid step uploads next link: %w", err)
			}
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
