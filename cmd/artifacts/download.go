package artifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
	ArtifactID  string `arg:"" optional:"" help:"Artifact ID to download. If omitted, all artifacts are downloaded. Use 'bk artifacts list' to find IDs."`
	BuildNumber string `help:"Build number containing the artifact. If omitted, the most recent build on the current branch will be used." short:"b" name:"build"`
	Pipeline    string `help:"The pipeline containing the artifact. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}. If omitted, it will be resolved using the current directory." short:"p"`
	JobUUID     string `help:"The job UUID containing the artifact." short:"j" name:"job-uuid"`
}

func (c *DownloadCmd) Help() string {
	return `
Use this command to download artifacts from a build.

If no artifact ID is provided, all artifacts for the build (or job) will be downloaded.
Artifact IDs can be found using "bk artifacts list".

Examples:
  # Download all artifacts from the most recent build on the current branch
  $ bk artifacts download

  # Download all artifacts from a specific build
  $ bk artifacts download --build 429

  # Download all artifacts from a specific job
  $ bk artifacts download --build 429 --job-uuid 0193903e-ecd9-4c51-9156-0738da987e87

  # Download a specific artifact
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c --build 429

  # Specify the pipeline explicitly
  $ bk artifacts download --build 429 -p monolith
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

	build := strconv.Itoa(bld.BuildNumber)

	if c.ArtifactID != "" {
		return c.downloadOne(ctx, f, bld.Organization, bld.Pipeline, build)
	}

	return c.downloadAll(ctx, f, bld.Organization, bld.Pipeline, build)
}

func (c *DownloadCmd) downloadOne(ctx context.Context, f *factory.Factory, org, pipeline, build string) error {
	var filename string

	if err := bkIO.SpinWhile(f, "Downloading artifact", func() error {
		artifact, findErr := findArtifact(ctx, f, org, pipeline, build, c.ArtifactID, c.JobUUID)
		if findErr != nil {
			return findErr
		}
		var dlErr error
		filename, dlErr = downloadArtifact(ctx, f, artifact)
		return dlErr
	}); err != nil {
		return err
	}

	fmt.Printf("Downloaded: %s\n", filename)
	return nil
}

func (c *DownloadCmd) downloadAll(ctx context.Context, f *factory.Factory, org, pipeline, build string) error {
	var artifacts []buildkite.Artifact

	if err := bkIO.SpinWhile(f, "Loading artifacts", func() error {
		var err error
		artifacts, err = listArtifacts(ctx, f, org, pipeline, build, c.JobUUID)
		return err
	}); err != nil {
		return err
	}

	if len(artifacts) == 0 {
		fmt.Println("No artifacts found.")
		return nil
	}

	directory := fmt.Sprintf("artifacts-build-%s", build)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		return err
	}

	for i := range artifacts {
		a := &artifacts[i]
		destPath := filepath.Join(directory, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
			return err
		}

		out, err := os.Create(destPath)
		if err != nil {
			return err
		}

		_, err = f.RestAPIClient.Artifacts.DownloadArtifactByURL(ctx, a.DownloadURL, out)
		out.Close()
		if err != nil {
			return err
		}

		fmt.Printf("Downloaded: %s\n", a.Path)
	}

	fmt.Printf("Downloaded %d artifacts to: %s\n", len(artifacts), directory)
	return nil
}

func findArtifact(ctx context.Context, f *factory.Factory, org, pipeline, build, artifactID, jobUUID string) (*buildkite.Artifact, error) {
	artifacts, err := listArtifacts(ctx, f, org, pipeline, build, jobUUID)
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

// listArtifacts fetches all artifacts for a build or job, paginating through all results.
func listArtifacts(ctx context.Context, f *factory.Factory, org, pipeline, build, jobUUID string) ([]buildkite.Artifact, error) {
	var all []buildkite.Artifact
	opts := &buildkite.ArtifactListOptions{
		ListOptions: buildkite.ListOptions{PerPage: 100},
	}

	for {
		var artifacts []buildkite.Artifact
		var resp *buildkite.Response
		var err error

		if jobUUID != "" {
			artifacts, resp, err = f.RestAPIClient.Artifacts.ListByJob(ctx, org, pipeline, build, jobUUID, opts)
		} else {
			artifacts, resp, err = f.RestAPIClient.Artifacts.ListByBuild(ctx, org, pipeline, build, opts)
		}
		if err != nil {
			return nil, err
		}

		all = append(all, artifacts...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

func downloadArtifact(ctx context.Context, f *factory.Factory, artifact *buildkite.Artifact) (string, error) {
	filename := filepath.Base(artifact.Path)

	out, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = f.RestAPIClient.Artifacts.DownloadArtifactByURL(ctx, artifact.DownloadURL, out)
	if err != nil {
		return "", err
	}

	return filename, nil
}
