package artifact

import (
	"context"
	"os"
	"path/filepath"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

type downloadContextKey struct{}

// IsDownload reports whether ctx belongs to an artifact payload request.
// HTTP redirects preserve this context, so debug logging can omit payload
// bodies without relying on their URL or content type.
func IsDownload(ctx context.Context) bool {
	marked, _ := ctx.Value(downloadContextKey{}).(bool)
	return marked
}

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

	ctx = context.WithValue(ctx, downloadContextKey{}, true)
	_, err = client.Artifacts.DownloadArtifactByURL(ctx, url, out)
	return err
}
