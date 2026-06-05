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

type RetryCmd struct {
	JobID       string `arg:"" help:"Job UUID to retry"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}" short:"p"`
	BuildNumber string `help:"The build number" short:"b"`
}

func (c *RetryCmd) Help() string {
	return `Use this command to retry build jobs.

Examples:
  # Retry a job (requires --pipeline and --build)
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31 -p my-pipeline -b 123

  # If inside a git repository with a configured pipeline
  $ bk job retry 0190046e-e199-453b-a302-a21a4d649d31 -b 123
`
}

func (c *RetryCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
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
	if err = bkIO.SpinWhile(f, "Retrying job", func() error {
		var apiErr error
		job, _, apiErr = f.RestAPIClient.Jobs.RetryJob(
			ctx,
			bld.Organization,
			bld.Pipeline,
			fmt.Sprint(bld.BuildNumber),
			c.JobID,
		)
		return apiErr
	}); err != nil {
		return err
	}

	fmt.Println("Successfully retried job: " + job.WebURL)
	return nil
}
