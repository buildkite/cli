package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type DownloadCmd struct {
	BuildNumber    string `arg:"" optional:"" help:"Build number to download (omit for most recent build)"`
	Pipeline       string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch         string `help:"Filter builds to this branch." short:"b"`
	User           string `help:"Filter builds to this user. You can use name or email." short:"u" xor:"userfilter"`
	Mine           bool   `help:"Filter builds to only my user." short:"m" xor:"userfilter"`
	ArtifactsPath  string `help:"Filter artifacts by path. Supports exact matches and glob patterns using * as a wildcard, e.g. --artifacts-path \"log/rspec*.json\"."`
	ArtifactsState string `help:"Filter artifacts to download by state. Must be one of: new, finished, error, deleted, expired."`
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
  $ bk build download --pipeline my-pipeline --mine

  # Filter artifacts to download by path or state
  $ bk build download --pipeline my-pipeline --artifacts-path "log/rspec*.json"
  $ bk build download --pipeline my-pipeline --artifacts-state finished
`
}

func (c *DownloadCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	if err := artifact.ValidateState(c.ArtifactsState); err != nil {
		return err
	}

	ctx := context.Background()

	// we find the pipeline based on the following rules:
	// 1. an explicit flag is passed
	// 2. a configured pipeline for this directory
	// 3. find pipelines matching the current repository from the API
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
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

	var (
		dir             string
		artifactMatches int
	)
	if err = bkIO.SpinWhile(f, "Downloading build resources", func() error {
		dir, artifactMatches, err = download(ctx, bld, c.ArtifactsPath, c.ArtifactsState, f)
		return err
	}); err != nil {
		return err
	}

	// Warn — but do not fail — when a user-supplied filter matched zero
	// artifacts. Other build resources (logs, metadata) were still downloaded.
	if artifactMatches == 0 && (c.ArtifactsPath != "" || c.ArtifactsState != "") {
		warnUnmatchedArtifactFilter(os.Stderr, c.ArtifactsPath, c.ArtifactsState)
	}

	fmt.Printf("Downloaded build to: %s\n", dir)

	return nil
}

// warnUnmatchedArtifactFilter writes a stderr warning when --artifacts-path
// or --artifacts-state was set but the API returned no matching artifacts.
// The build download itself still succeeds; the message just makes the empty
// filter visible so users don't wonder why their build directory lacks the
// artifact files they expected.
func warnUnmatchedArtifactFilter(w io.Writer, path, state string) {
	switch {
	case path != "" && state != "":
		fmt.Fprintf(w, "Warning: no artifacts matched path %q and state %q.\n", path, state)
	case path != "":
		fmt.Fprintf(w, "Warning: no artifacts matched path %q.\n", path)
	case state != "":
		fmt.Fprintf(w, "Warning: no artifacts matched state %q.\n", state)
	}
}

// download returns the destination directory and the number of artifacts the
// (optional) filter matched. A zero count with a filter set is not itself an
// error — the caller decides how to surface it.
//
// State-value validation lives in Run() and inside artifact.List, so an
// invalid --artifacts-state still surfaces as a validation error without
// hitting either endpoint here.
func download(ctx context.Context, bld *build.Build, artifactsPath, artifactsState string, f *factory.Factory) (string, int, error) {
	// Jobs are needed for log downloads, but the pipeline payload is unused.
	getOpts := &buildkite.BuildGetOptions{
		BuildsListOptions: buildkite.BuildsListOptions{ExcludePipeline: true},
	}
	b, _, err := f.RestAPIClient.Builds.Get(ctx, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), getOpts)
	if err != nil {
		return "", 0, err
	}

	directory := fmt.Sprintf("build-%s", b.ID)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		return "", 0, err
	}

	// Paginate the artifact list up front so every matching artifact gets
	// downloaded, not just the first page.
	artifacts, err := artifact.List(ctx, f.RestAPIClient, bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), "", artifactsPath, artifactsState)
	if err != nil {
		return "", 0, err
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	// recordErr keeps the first error we see across all worker goroutines,
	// so the caller sees a real cause rather than a random race winner.
	recordErr := func(e error) {
		if e == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = e
		}
		mu.Unlock()
	}

	for _, job := range b.Jobs {
		// only script (command) jobs will have logs
		if job.Type != "script" {
			continue
		}

		wg.Add(1)
		go func(jobID string) {
			defer wg.Done()

			log, _, apiErr := f.RestAPIClient.Jobs.GetJobLog(ctx, bld.Organization, bld.Pipeline, b.ID, jobID)
			if apiErr != nil {
				recordErr(apiErr)
				return
			}

			if writeErr := os.WriteFile(filepath.Join(directory, jobID), []byte(log.Content), 0o644); writeErr != nil {
				recordErr(writeErr)
			}
		}(job.ID)
	}

	for _, a := range artifacts {
		wg.Add(1)
		go func(a buildkite.Artifact) {
			defer wg.Done()

			// Keep the historical flat layout: build-<uuid>/artifact-<id>-<filename>.
			// This is UX-observable and separate from `bk artifacts download`,
			// which mirrors the artifact's own directory structure.
			dest := filepath.Join(directory, fmt.Sprintf("artifact-%s-%s", a.ID, a.Filename))
			if dlErr := artifact.DownloadToFile(ctx, f.RestAPIClient, a.DownloadURL, dest); dlErr != nil {
				recordErr(dlErr)
			}
		}(a)
	}

	wg.Wait()
	if firstErr != nil {
		return "", len(artifacts), firstErr
	}

	return directory, len(artifacts), nil
}
