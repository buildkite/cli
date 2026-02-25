package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	var p buildkite.Pipeline
	spinErr := bkIO.SpinWhile(f, "Loading pipeline information", func() {
		p, _, err = f.RestAPIClient.Pipelines.Get(ctx, pipeline.Org, pipeline.Name)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	pipelineView := output.Viewable[buildkite.Pipeline]{
		Data:   p,
		Render: renderPipelineText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, pipelineView, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	return output.Write(writer, pipelineView, format)
}

func renderPipelineText(p buildkite.Pipeline) string {
	rows := [][]string{
		{"Description", output.ValueOrDash(p.Description)},
		{"Repository", output.ValueOrDash(p.Repository)},
		{"Default Branch", output.ValueOrDash(p.DefaultBranch)},
		{"Visibility", output.ValueOrDash(p.Visibility)},
		{"Web URL", output.ValueOrDash(p.WebURL)},
	}

	if len(p.Tags) > 0 {
		rows = append(rows, []string{"Tags", strings.Join(p.Tags, ", ")})
	}

	if p.ClusterID != "" {
		rows = append(rows, []string{"Cluster ID", p.ClusterID})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Viewing %s\n\n", output.ValueOrDash(p.Name))

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)

	if p.Configuration != "" {
		sb.WriteString("\n\nConfiguration:\n")
		sb.WriteString(p.Configuration)
	}

	return sb.String()
}
