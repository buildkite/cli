package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// FindCluster resolves a cluster for the given org. If clusterID is provided,
// it is returned directly. Otherwise it looks up the "Default" cluster.
func FindCluster(ctx context.Context, f *factory.Factory, org, clusterID string) (string, error) {
	if clusterID != "" {
		return clusterID, nil
	}

	clusters, _, err := f.RestAPIClient.Clusters.List(ctx, org, nil)
	if err != nil {
		return "", err
	}

	for _, c := range clusters {
		if c.Name == "Default" {
			return c.ID, nil
		}
	}

	return "", fmt.Errorf("no cluster named \"Default\" found in organization %q", org)
}

// CreateAgentToken creates an agent token on the given cluster and returns the token string.
func CreateAgentToken(ctx context.Context, f *factory.Factory, org, clusterID, description string) (string, error) {
	token, _, err := f.RestAPIClient.ClusterTokens.Create(ctx, org, clusterID, buildkite.ClusterTokenCreateUpdate{
		Description: description,
	})
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

// WriteAgentConfig writes a minimal agent config file with the given token, build path,
// and optional tags. The file is created with 0600 permissions.
func WriteAgentConfig(path, token, buildPath string, tags []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := fmt.Sprintf("token=%q\nbuild-path=%q\n", token, buildPath)
	for _, tag := range tags {
		content += fmt.Sprintf("tags=%q\n", tag)
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
