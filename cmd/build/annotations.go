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

type AnnotationsCmd struct {
	BuildNumber string `arg:"" help:"Build number, org/pipeline/number, or Buildkite build URL"`
	Pipeline    string `help:"Pipeline slug or org/pipeline. Inferred from configuration or repository if omitted." short:"p"`
	output.OutputFlags
}

func (c *AnnotationsCmd) Help() string {
	return `List all build- and job-scoped annotations for an explicit build.
All pages are fetched, including HTML bodies (not the original Markdown).
Requires the read_builds token scope. JSON/YAML never prompts for pipeline
selection; use --pipeline when ambiguous. Empty results have annotations=[].
Errors return a nonzero exit status without partial results on stdout.

Examples:
  $ bk build annotations 123 --pipeline my-pipeline --json
  $ bk build annotations acme/my-pipeline/123 --text
  $ bk build annotations https://buildkite.com/acme/my-pipeline/builds/123 --yaml
  $ bk build annotations acme/my-pipeline/123 --json | jq '.annotations[].body_html'
`
}

// Keep REST field names consistent in JSON and YAML; the SDK only tags JSON.
type annotationOutput struct {
	ID        string               `json:"id,omitempty" yaml:"id,omitempty"`
	Context   string               `json:"context,omitempty" yaml:"context,omitempty"`
	Style     string               `json:"style,omitempty" yaml:"style,omitempty"`
	Scope     string               `json:"scope,omitempty" yaml:"scope,omitempty"`
	JobID     string               `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Priority  int                  `json:"priority,omitempty" yaml:"priority,omitempty"`
	BodyHTML  string               `json:"body_html,omitempty" yaml:"body_html,omitempty"`
	CreatedAt *buildkite.Timestamp `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt *buildkite.Timestamp `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type annotationsOutput struct {
	Organization string             `json:"organization" yaml:"organization"`
	Pipeline     string             `json:"pipeline" yaml:"pipeline"`
	BuildNumber  int                `json:"build_number" yaml:"build_number"`
	Annotations  []annotationOutput `json:"annotations" yaml:"annotations"`
}

func (r annotationsOutput) TextOutput() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Annotations for %s/%s/%d (%d)\n", r.Organization, r.Pipeline, r.BuildNumber, len(r.Annotations))
	if len(r.Annotations) == 0 {
		sb.WriteString("\nNo annotations found.\n")
	}
	for _, annotation := range r.Annotations {
		fmt.Fprintf(&sb, "\n%s [%s] (ID: %s, scope: %s)\n", annotation.Context, annotation.Style, annotation.ID, annotation.Scope)
		if annotation.JobID != "" {
			fmt.Fprintf(&sb, "Job: %s\n", annotation.JobID)
		}
		fmt.Fprintf(&sb, "%s\n", annotation.BodyHTML)
	}
	return sb.String()
}

func (c *AnnotationsCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	// Pipeline prompts write to stdout, so structured output must not prompt.
	f.NoInput = f.NoInput || globals.DisableInput() || format != output.FormatText
	f.Quiet = f.Quiet || globals.IsQuiet()
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)
	ctx := context.Background()
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
	f.Quiet = f.Quiet || globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), bld.Organization); err != nil {
		return err
	}

	result := annotationsOutput{
		Organization: bld.Organization,
		Pipeline:     bld.Pipeline,
		BuildNumber:  bld.BuildNumber,
		Annotations:  []annotationOutput{},
	}
	if err := bkIO.SpinWhile(f, "Loading annotations", func() error {
		opts := &buildkite.AnnotationListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: pageSize},
			Scope:       "all",
		}
		for {
			annotations, resp, err := f.RestAPIClient.Annotations.ListByBuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), opts)
			if err != nil {
				return fmt.Errorf("failed to list annotations (requires read_builds): %w", err)
			}
			for _, annotation := range annotations {
				result.Annotations = append(result.Annotations, annotationOutput(annotation))
			}
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
