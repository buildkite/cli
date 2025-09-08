package build

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Get the command's required and optional scopes
			cmdScopes := scopes.GetCommandScopes(cmd)

			// Get the token scopes from the factory
			tokenScopes := f.Config.GetTokenScopes()
			if len(tokenScopes) == 0 {
				return fmt.Errorf("no scopes found in token. Please ensure you're using a token with appropriate scopes")
			}

			// Validate the scopes
			if err := scopes.ValidateScopes(cmdScopes, tokenScopes); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get pipeline from persistent flag
			pipeline, _ = cmd.Flags().GetString("pipeline")

			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}

			err = bk_io.Confirm(&confirmed, fmt.Sprintf("Cancel build #%d on %s", bld.BuildNumber, bld.Pipeline))
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

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.WriteBuilds),
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been cancelled.")
	// Pipeline flag now inherited from parent command
	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt. Useful if being used in automation/CI.")
	cmd.Flags().SortFlags = false

	return &cmd
}

func cancelBuild(ctx context.Context, org string, pipeline string, buildId string, web bool, f *factory.Factory) error {
	var err error
	var build buildkite.Build
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Cancelling build #%s from pipeline %s", buildId, pipeline), func() {
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
