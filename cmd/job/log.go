package job

import (
	"context"
	"fmt"
	"regexp"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type LogCmd struct {
	JobID        string `arg:"" help:"Job UUID to get logs for"`
	Pipeline     string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	BuildNumber  string `help:"The build number." short:"b"`
	NoTimestamps bool   `help:"Strip timestamp prefixes from log output." name:"no-timestamps"`
}

func (c *LogCmd) Help() string {
	return `
Examples:
  # Get a job's logs by UUID (requires --pipeline and --build)
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 -p my-pipeline -b 123

  # If inside a git repository with a configured pipeline
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 -b 123

  # Strip timestamp prefixes from output
  $ bk job log 0190046e-e199-453b-a302-a21a4d649d31 -p my-pipeline -b 123 --no-timestamps
`
}

func (c *LogCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

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

	var logContent string
	err = bkIO.SpinWhile(f, "Fetching job log", func() {
		jobLog, _, apiErr := f.RestAPIClient.Jobs.GetJobLog(
			ctx,
			bld.Organization,
			bld.Pipeline,
			fmt.Sprint(bld.BuildNumber),
			c.JobID,
		)
		if apiErr != nil {
			err = apiErr
			return
		}
		logContent = jobLog.Content
	})
	if err != nil {
		return err
	}

	if c.NoTimestamps {
		logContent = stripTimestamps(logContent)
	}

	writer, cleanup := bkIO.Pager(f.NoPager)
	defer func() { _ = cleanup() }()

	fmt.Fprint(writer, logContent)
	return nil
}

var timestampRegex = regexp.MustCompile(`bk;t=\d+\x07`)

func stripTimestamps(content string) string {
	return timestampRegex.ReplaceAllString(content, "")
}
