package build

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/cli"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CancelCmd struct {
	BuildNumber string `arg:"" help:"Build number to cancel"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Web         bool   `help:"Open the build in a web browser after it has been cancelled." short:"w"`
}

func (c *CancelCmd) Help() string {
	return `
Examples:
  # Cancel a build by number
  $ bk build cancel 123 --pipeline my-pipeline

  # Cancel a build and open in browser
  $ bk build cancel 123 -pipeline my-pipeline --web`
}

func (c *CancelCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
	)

	args := []string{c.BuildNumber}
	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
	)

	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}

	confirmed, err := bk_io.Confirm(f, fmt.Sprintf("Cancel build #%d on %s", bld.BuildNumber, bld.Pipeline))
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	return cancelBuild(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), c.Web, f)
}

func cancelBuild(ctx context.Context, org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	var err error
	var build buildkite.Build
	spinErr := bk_io.SpinWhile(f, fmt.Sprintf("Cancelling build #%s from pipeline %s", buildId, pipeline), func() {
		build, err = f.RestAPIClient.Builds.Cancel(ctx, org, pipeline, buildId)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", renderResult(fmt.Sprintf("Build canceled: %s", build.WebURL)))

	return util.OpenInWebBrowser(web, build.WebURL)
}
