package build

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

func NewCmdBuildCancel(f *factory.Factory) *cobra.Command {
	var web bool
	var pipeline string
	var confirmed bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "cancel <number> [flags]",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Cancel a build.",
		Long: heredoc.Doc(`
			Cancel the given build.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}

			err = io.Confirm(&confirmed, fmt.Sprintf("Cancel build #%d on %s", bld.BuildNumber, bld.Pipeline))
			if err != nil {
				return err
			}

			if confirmed {
				return cancelBuild(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), web, f)
			} else {
				return nil
			}
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been cancelled.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to cancel a build on. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt. Useful if being used in automation/CI.")
	cmd.Flags().SortFlags = false

	return &cmd
}

func cancelBuild(ctx context.Context, org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	var err error
	var build buildkite.Build
	spinErr := spinner.New().
		Title(fmt.Sprintf("Cancelling build #%s from pipeline %s", buildId, pipeline)).
		Action(func() {
			build, err = f.RestAPIClient.Builds.Cancel(ctx, org, pipeline, buildId)
		}).
		Run()
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", renderResult(fmt.Sprintf("Build canceled: %s", build.WebURL)))

	return util.OpenInWebBrowser(web, build.WebURL)
}
