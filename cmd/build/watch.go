package build

import (
	"context"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/build/view/shared"
	"github.com/buildkite/cli/v3/internal/cli"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pkgValidation "github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type WatchCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to watch (omit for most recent build)"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string `help:"The branch to watch builds for." short:"b"`
	Interval    int    `help:"Polling interval in seconds" default:"1"`
}

func (c *WatchCmd) Help() string {
	return `
Examples:
  # Watch the most recent build for the current branch
  $ bk build watch --pipeline my-pipeline

  # Watch a specific build
  $ bk build watch 429 --pipeline my-pipeline

  # Watch the most recent build on a specific branch
  $ bk build watch -b feature-x --pipeline my-pipeline

  # Watch a build on a specific pipeline
  $ bk build watch --pipeline my-pipeline

  # Set a custom polling interval (in seconds)
  $ bk build watch --interval 5 --pipeline my-pipeline`
}

func (c *WatchCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := pkgValidation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	// Validate command options
	v := validation.New()
	v.AddRule("Interval", validation.MinValue(1))
	if c.Pipeline != "" {
		v.AddRule("Pipeline", validation.Slug)
	}
	if err := v.Validate(map[string]interface{}{
		"Pipeline": c.Pipeline,
		"Interval": c.Interval,
	}); err != nil {
		return err
	}

	ctx := context.Background()

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
	)

	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(c.Branch),
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
		return fmt.Errorf("no running builds found")
	}

	fmt.Printf("Watching build %d on %s/%s\n", bld.BuildNumber, bld.Organization, bld.Pipeline)

	ticker := time.NewTicker(time.Duration(c.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b, _, err := f.RestAPIClient.Builds.Get(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
			if err != nil {
				return err
			}

			summary := shared.BuildSummaryWithJobs(&b)
			fmt.Printf("\033[2J\033[H%s\n", summary) // Clear screen and move cursor to top-left

			if b.FinishedAt != nil {
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}
