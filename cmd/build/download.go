package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type DownloadCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to download (omit for most recent build)"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string `help:"Filter builds to this branch." short:"b"`
	User        string `help:"Filter builds to this user. You can use name or email." short:"u" xor:"userfilter"`
	Mine        bool   `help:"Filter builds to only my user." short:"m" xor:"userfilter"`
}

func (c *DownloadCmd) Help() string {
	return `
Examples:
  # Download build 123
  $ bk build download 123 --pipeline my-pipeline

  # Download most recent build
  $ bk build download --pipeline my-pipeline

  # Download most recent build on a branch
  $ bk build download -b main --pipeline my-pipeline

  # Download most recent build by a user
  $ bk build download --pipeline my-pipeline -u alice@hello.com

  # Download most recent build by yourself
  $ bk build download --pipeline my-pipeline --mine`
}

func (c *DownloadCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
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

	var dir string
	spinErr := bkIO.SpinWhile(f, "Downloading build resources", func() {
		dir, err = download(ctx, bld, f)
	})
	if spinErr != nil {
		return spinErr
	}

	fmt.Printf("Downloaded build to: %s\n", dir)

	return err
}

func download(ctx context.Context, build *build.Build, f *factory.Factory) (string, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	b, _, err := f.RestAPIClient.Builds.Get(ctx, build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return "", err
	}

	directory := fmt.Sprintf("build-%s", b.ID)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return "", err
	}

	for _, job := range b.Jobs {
		// only script (command) jobs will have logs
		if job.Type != "script" {
			continue
		}

		go func() {
			defer wg.Done()
			wg.Add(1)
			log, _, apiErr := f.RestAPIClient.Jobs.GetJobLog(ctx, build.Organization, build.Pipeline, b.ID, job.ID)
			if err != nil {
				mu.Lock()
				err = apiErr
				mu.Unlock()
				return
			}

			fileErr := os.WriteFile(filepath.Join(directory, job.ID), []byte(log.Content), 0o644)
			if fileErr != nil {
				mu.Lock()
				err = fileErr
				mu.Unlock()
			}
		}()
	}

	artifacts, _, err := f.RestAPIClient.Artifacts.ListByBuild(ctx, build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return "", err
	}

	for _, artifact := range artifacts {
		go func() {
			defer wg.Done()
			wg.Add(1)
			out, fileErr := os.Create(filepath.Join(directory, fmt.Sprintf("artifact-%s-%s", artifact.ID, artifact.Filename)))
			if err != nil {
				err = fileErr
			}
			_, apiErr := f.RestAPIClient.Artifacts.DownloadArtifactByURL(ctx, artifact.DownloadURL, out)
			if err != nil {
				err = apiErr
			}
		}()
	}

	wg.Wait()
	if err != nil {
		return "", err
	}

	return directory, nil
}
