package artifacts

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/build/resolver"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdArtifacts(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "artifacts <command>",
		Args:  cobra.ArbitraryArgs,
		Long:  "Manage pipeline build artifacts",
		Short: "Manage pipeline build artifacts",
		Example: heredoc.Doc(`
			# To view pipeline build artifacts
			$ bk artifacts list -b "build number"
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
	}
	cmd.AddCommand(NewCmdArtifactsList(f))

	return &cmd
}

func resolveFrom(pipeline string, f *factory.Factory, args []string) resolver.AggregateResolver {
	//resolve a pipeline based on how bk build resolves the pipeline
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
	)

	// we resolve a build  an optional argument or positional argument
	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)
	return buildRes
}
