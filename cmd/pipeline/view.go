package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	view "github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/pkg/browser"
)

type ViewCmd struct {
	Pipeline string `arg:"" help:"The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." optional:""`
	Web      bool   `help:"Open the pipeline in a web browser." short:"w"`
	Output   string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *ViewCmd) Help() string {
	return `View information about a pipeline.

Examples:
  # View a pipeline
  $ bk pipeline view my-pipeline

  # View a pipeline in a specific organization
  $ bk pipeline view my-org/my-pipeline

  # Open pipeline in browser
  $ bk pipeline view my-pipeline --web

  # Output as JSON
  $ bk pipeline view my-pipeline -o json
`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	var args []string
	if c.Pipeline != "" {
		args = []string{c.Pipeline}
	}

	pipelineRes := resolver.NewAggregateResolver(
		resolver.ResolveFromPositionalArgument(args, 0, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f))),
	)

	pipeline, err := pipelineRes.Resolve(ctx)
	if err != nil {
		return err
	}

	slug := fmt.Sprintf("%s/%s", pipeline.Org, pipeline.Name)

	if c.Web {
		return browser.OpenURL(fmt.Sprintf("https://buildkite.com/%s", slug))
	}

	resp, err := graphql.GetPipeline(ctx, f.GraphQLClient, slug)
	if err != nil {
		return err
	}
	if resp == nil || resp.Pipeline == nil {
		fmt.Printf("Could not find pipeline: %s\n", slug)
		return nil
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	if format != output.FormatText {
		return output.Write(kongCtx.Stdout, resp.Pipeline, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	var pipelineOutput strings.Builder

	err = view.RenderPipeline(&pipelineOutput, *resp.Pipeline)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(writer, "%s\n", pipelineOutput.String())
	return err
}
