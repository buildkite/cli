package job

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ReprioritizeCmd struct {
	JobID       string `arg:"" help:"Job UUID to reprioritize"`
	Priority    int    `arg:"" help:"New priority value for the job"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}" short:"p"`
	BuildNumber string `help:"The build number" short:"b"`
}

func (c *ReprioritizeCmd) Help() string {
	return `
Examples:
  # Reprioritize a job (requires --pipeline and --build)
  $ bk job reprioritize 0190046e-e199-453b-a302-a21a4d649d31 1 -p my-pipeline -b 123

  # If inside a git repository with a configured pipeline
  $ bk job reprioritize 0190046e-e199-453b-a302-a21a4d649d31 1 -b 123
`
}

func (c *ReprioritizeCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f), f.GitRepository != nil)),
	)

	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	args := []string{}
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}
	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)

	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}
	if bld == nil {
		return fmt.Errorf("no build found")
	}

	var job buildkite.Job
	err = bkIO.SpinWhile(f, "Reprioritizing job", func() {
		job, _, err = f.RestAPIClient.Jobs.ReprioritizeJob(
			ctx,
			bld.Organization,
			bld.Pipeline,
			fmt.Sprint(bld.BuildNumber),
			c.JobID,
			&buildkite.JobReprioritizationOptions{
				Priority: c.Priority,
			},
		)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Job reprioritized to %d: %s\n", c.Priority, job.WebURL)
	return nil
}
