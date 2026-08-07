package artifact

import (
	"context"
	"os"
	"path/filepath"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

// DownloadToFile creates destPath (including any missing parent directories)
// and streams the artifact at url into it.
func DownloadToFile(ctx context.Context, client *buildkite.Client, url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = client.Artifacts.DownloadArtifactByURL(ctx, url, out)
	return err
}
