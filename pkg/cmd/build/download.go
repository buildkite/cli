package build

import (
	"fmt"

	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdBuildDownload(f *factory.Factory) *cobra.Command {
	var logs bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download [number [pipeline]] [flags]",
		Short:                 "Download resources of a build",
		Long:                  "Download allows you to download resources from a build.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromPositionalArgument(args, 1, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				// buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("could not resolve a build")
			}

			return download()
		},
	}

	cmd.Flags().BoolVar(&logs, "logs", false, "Download all job logs for the build")

	return &cmd
}

func download() error {
	return nil
}
