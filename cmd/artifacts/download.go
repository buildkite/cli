package artifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type DownloadCmd struct {
	ArtifactID  string `arg:"" help:"Artifact ID to download (use 'bk artifacts list' to find IDs)"`
	BuildNumber string `help:"Build number containing the artifact. If omitted, the most recent build on the current branch will be used." short:"b" name:"build"`
	Pipeline    string `help:"The pipeline containing the artifact. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}. If omitted, it will be resolved using the current directory." short:"p"`
	Job         string `help:"The job containing the artifact." short:"j"`
}

func (c *DownloadCmd) Help() string {
	return `
Use this command to download a specific artifact from a build.

The artifact ID can be found using "bk artifacts list".

Examples:
  # Download an artifact from the most recent build on the current branch
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c

  # Download an artifact from a specific build
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c --build 429

  # Download an artifact from a specific job
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c --build 429 --job 0193903e-ecd9-4c51-9156-0738da987e87

  # Specify the pipeline explicitly
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c --build 429 -p monolith
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

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)

	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(""),
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	var buildResolvers []buildResolver.BuildResolverFn
	if c.BuildNumber != "" {
		buildResolvers = append(buildResolvers, buildResolver.ResolveFromPositionalArgument([]string{c.BuildNumber}, 0, pipelineRes.Resolve, f.Config))
	}
	buildResolvers = append(buildResolvers, buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...))

	buildRes := buildResolver.NewAggregateResolver(buildResolvers...)

	ctx := context.Background()
	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}
	if bld == nil {
		return bkErrors.NewResourceNotFoundError(nil, "no build found")
	}

	build := fmt.Sprint(bld.BuildNumber)
	var downloadDir string

	if err = bkIO.SpinWhile(f, "Downloading artifact", func() error {
		artifact, findErr := findArtifact(ctx, f, bld.Organization, bld.Pipeline, build, c.ArtifactID, c.Job)
		if findErr != nil {
			return findErr
		}
		downloadDir, err = downloadArtifact(ctx, f, artifact, c.ArtifactID)
		return err
	}); err != nil {
		return err
	}

	fmt.Printf("Downloaded artifact to: %s\n", downloadDir)
	return nil
}

func findArtifact(ctx context.Context, f *factory.Factory, org, pipeline, build, artifactID, jobID string) (*buildkite.Artifact, error) {
	var artifacts []buildkite.Artifact
	var err error

	if jobID != "" {
		artifacts, _, err = f.RestAPIClient.Artifacts.ListByJob(ctx, org, pipeline, build, jobID, nil)
	} else {
		artifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(ctx, org, pipeline, build, nil)
	}
	if err != nil {
		return nil, err
	}

	for i := range artifacts {
		if artifacts[i].ID == artifactID {
			return &artifacts[i], nil
		}
	}

	return nil, bkErrors.NewResourceNotFoundError(nil, fmt.Sprintf("no artifact found with ID %s in build #%s", artifactID, build))
}

func downloadArtifact(ctx context.Context, f *factory.Factory, artifact *buildkite.Artifact, artifactID string) (string, error) {
	directory := fmt.Sprintf("artifact-%s", artifactID)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		return "", err
	}

	filename := filepath.Base(artifact.Path)
	out, err := os.Create(filepath.Join(directory, filename))
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = f.RestAPIClient.Artifacts.DownloadArtifactByURL(ctx, artifact.DownloadURL, out)
	if err != nil {
		return "", err
	}

	return directory, nil
}
