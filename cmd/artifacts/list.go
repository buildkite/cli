package artifacts

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/artifact"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ListCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to list artifacts for"`
	Pipeline    string `help:"The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}. If omitted, it will be resolved using the current directory." short:"p"`
	Job         string `help:"List artifacts for a specific job on the given build." short:"j"`
	output.OutputFlags
}

func (c *ListCmd) Help() string {
	return `
List artifacts for a build or a job in a build.

You can pass an optional build number. If omitted, the most recent build on the current branch will be resolved.

Examples:
  # By default, artifacts of the most recent build for the current branch is shown
  $ bk artifacts list

  # To list artifacts of a specific build
  $ bk artifacts list 429

  # To list artifacts of a specific job in a build
  $ bk artifacts list 429 --job 0193903e-ecd9-4c51-9156-0738da987e87

  # If not inside a repository or to use a specific pipeline, pass -p
  $ bk artifacts list 429 -p monolith
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	var args []string
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}

	// Resolve a pipeline based on how bk build resolves the pipeline
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)

	// We resolve a build an optional argument or positional argument
	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(""),
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)

	ctx := context.Background()
	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}
	if bld == nil {
		fmt.Println("No build found.")
		return nil
	}

	var buildArtifacts []buildkite.Artifact

	err = bkIO.SpinWhile(f, "Loading artifacts information", func() {
		if c.Job != "" {
			buildArtifacts, _, err = f.RestAPIClient.Artifacts.ListByJob(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), c.Job, nil)
		} else {
			buildArtifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
		}
	})
	if err != nil {
		return err
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, buildArtifacts, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	if len(buildArtifacts) == 0 {
		fmt.Fprintln(writer, "No artifacts found.")
		return nil
	}

	buildURL := fmt.Sprintf("https://buildkite.com/organizations/%s/pipelines/%s/builds/%d", bld.Organization, bld.Pipeline, bld.BuildNumber)

	if c.Job != "" {
		jobURL := fmt.Sprintf("%s/jobs/%s", buildURL, c.Job)
		fmt.Fprintf(writer, "Showing %d artifacts for %s/%s build #%d (job %s): %s\n\n", len(buildArtifacts), bld.Organization, bld.Pipeline, bld.BuildNumber, c.Job, jobURL)
	} else {
		fmt.Fprintf(writer, "Showing %d artifacts for %s/%s build #%d: %s\n\n", len(buildArtifacts), bld.Organization, bld.Pipeline, bld.BuildNumber, buildURL)
	}

	return displayArtifacts(buildArtifacts, writer, buildURL)
}

func displayArtifacts(artifacts []buildkite.Artifact, writer io.Writer, baseBuildURL string) error {
	headers := []string{"ID", "Path", "Size", "URL"}
	var rows [][]string

	for _, a := range artifacts {
		url := "-"
		if a.JobID != "" {
			url = fmt.Sprintf("%s/jobs/%s/artifacts/%s", baseBuildURL, a.JobID, a.ID)
		} else if a.URL != "" {
			url = a.URL
		}
		rows = append(rows, []string{
			a.ID,
			a.Path,
			artifact.FormatBytes(a.FileSize),
			url,
		})
	}

	table := output.Table(headers, rows, map[string]string{
		"id":   "dim",
		"path": "bold",
		"size": "dim",
		"url":  "dim",
	})

	fmt.Fprint(writer, table)
	return nil
}
