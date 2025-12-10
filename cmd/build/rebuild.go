package build

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type RebuildCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to rebuild (omit for most recent build)"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string `help:"Filter builds to this branch." short:"b"`
	User        string `help:"Filter builds to this user. You can use name or email." short:"u" xor:"userfilter"`
	Mine        bool   `help:"Filter builds to only my user." short:"m" xor:"userfilter"`
	Web         bool   `help:"Open the build in a web browser after it has been created." short:"w"`
}

func (c *RebuildCmd) Help() string {
	return `
Examples:
  # Rebuild a specific build by number
  $ bk build rebuild 123

  # Rebuild most recent build
  $ bk build rebuild

  # Rebuild and open in browser
  $ bk build rebuild 123 --web

  # Rebuild most recent build on a branch
  $ bk build rebuild -b main

  # Rebuild most recent build by a user
  $ bk build rebuild -u alice

  # Rebuild most recent build by yourself
  $ bk build rebuild --mine`
}

func (c *RebuildCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	ctx := context.Background()

	// we find the pipeline based on the following rules:
	// 1. an explicit flag is passed
	// 2. a configured pipeline for this directory
	// 3. find pipelines matching the current repository from the API
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
	)

	// we resolve a build based on the following rules:
	// 1. an optional argument
	// 2. resolve from API using some context
	//    a. filter by branch if --branch or use current repo
	//    b. filter by user if --user or --mine given
	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(c.Branch),
		options.ResolveBranchFromRepository(f.GitRepository),
	}.WithResolverWhen(
		c.User != "",
		options.ResolveUserFromFlag(c.User),
	).WithResolverWhen(
		c.Mine || c.User == "",
		options.ResolveCurrentUser(ctx, f),
	)

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
		fmt.Println("No build found.")
		return nil
	}

	return rebuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), c.Web, f)
}

func rebuild(ctx context.Context, org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	var err error
	var build buildkite.Build
	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Rerunning build #%s for pipeline %s", buildId, pipeline), func() {
		build, err = f.RestAPIClient.Builds.Rebuild(ctx, org, pipeline, buildId)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", renderResult(fmt.Sprintf("Build created: %s", build.WebURL)))

	return util.OpenInWebBrowser(web, build.WebURL)
}
