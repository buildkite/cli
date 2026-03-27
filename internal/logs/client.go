package logs

import (
	"context"
	"os"

	buildkitelogs "github.com/buildkite/buildkite-logs"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// NewClient creates a buildkite-logs client using the provided REST API client.
// Cache storage defaults to ~/.bklog; override with the BKLOG_CACHE_URL env var.
func NewClient(ctx context.Context, restClient *buildkite.Client, opts ...buildkitelogs.ClientOption) (*buildkitelogs.Client, error) {
	storageURL := os.Getenv("BKLOG_CACHE_URL")
	return buildkitelogs.NewClient(ctx, restClient, storageURL, opts...)
}
