package artifacts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type DownloadCmd struct {
	ArtifactID string `arg:"" help:"Artifact UUID to download"`
}

func (c *DownloadCmd) Help() string {
	return `
Use this command to download a specific artifact.

Examples:
  # Download an artifact by UUID
  $ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c
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

	ctx := context.Background()
	var downloadDir string

	spinErr := bkIO.SpinWhile(f, "Downloading artifact", func() {
		downloadDir, err = download(ctx, f, c.ArtifactID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	fmt.Printf("Downloaded artifact to: %s\n", downloadDir)
	return nil
}

func download(ctx context.Context, f *factory.Factory, artifactID string) (string, error) {
	resp, err := bkGraphQL.GetArtifacts(ctx, f.GraphQLClient, artifactID)
	if err != nil {
		return "", err
	}

	if resp == nil || resp.Artifact == nil {
		return "", bkErrors.NewResourceNotFoundError(nil, fmt.Sprintf("no artifact found with ID: %s", artifactID))
	}

	directory := fmt.Sprintf("artifact-%s", artifactID)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(resp.Artifact.Path)
	out, fileErr := os.Create(filepath.Join(directory, filename))
	if fileErr != nil {
		return "", fileErr
	}
	defer out.Close()

	apiResp, apiErr := http.Get(resp.Artifact.DownloadURL)
	if apiErr != nil {
		return "", apiErr
	}
	defer apiResp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, apiResp.Body)
	if err != nil {
		return "", err
	}

	return directory, nil
}
