package artifacts

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/artifact"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

type ListCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to list artifacts for"`
	Pipeline    string `help:"The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}. If omitted, it will be resolved using the current directory." short:"p"`
	Job         string `help:"List artifacts for a specific job on the given build." short:"j"`
	Output      string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
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
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	format := output.Format(c.Output)
	if format != output.FormatJSON && format != output.FormatYAML && format != output.FormatText {
		return fmt.Errorf("invalid output format: %s", c.Output)
	}

	var args []string
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}

	// Resolve a pipeline based on how bk build resolves the pipeline
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
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

	var wg sync.WaitGroup
	var mu sync.Mutex

	err = bkIO.SpinWhile(f, "Loading artifacts information", func() {
		wg.Add(1)

		go func() {
			defer wg.Done()
			var apiErr error

			if c.Job != "" { // list artifacts for a specific job
				buildArtifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByJob(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), c.Job, nil)
			} else {
				buildArtifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByBuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
			}
			if apiErr != nil {
				mu.Lock()
				err = apiErr
				mu.Unlock()
			}
		}()

		wg.Wait()
	})
	if err != nil {
		return err
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, buildArtifacts, format)
	}

	var summary string
	if len(buildArtifacts) > 0 {
		summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("Artifacts")
		for _, a := range buildArtifacts {
			summary += artifact.ArtifactSummary(&a)
		}
	} else {
		summary += lipgloss.NewStyle().Padding(0, 1).Render("No artifacts found.")
	}

	fmt.Printf("%s\n", summary)
	return nil
}
