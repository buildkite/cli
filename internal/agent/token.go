package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

// FindCluster resolves an explicit UUID or exact name, falling back to "Default".
// UUIDs are passed through without a lookup. Resolution failures list all clusters
// visible to the caller so they can retry with an explicit selector.
func FindCluster(ctx context.Context, f *factory.Factory, org, clusterID, clusterName string) (string, error) {
	if clusterID != "" && clusterName != "" {
		return "", fmt.Errorf("only one of --cluster-uuid or --cluster-name can be specified")
	}
	if clusterID != "" {
		return clusterID, nil
	}
	if clusterName == "" {
		clusterName = "Default"
	}

	var clusters []buildkite.Cluster
	opts := buildkite.ClustersListOptions{ListOptions: buildkite.ListOptions{Page: 1, PerPage: 100}}
	for {
		page, resp, err := f.RestAPIClient.Clusters.List(ctx, org, &opts)
		if err != nil {
			return "", fmt.Errorf("listing clusters for organization %q: %w", org, err)
		}
		clusters = append(clusters, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	if len(clusters) == 0 {
		return "", fmt.Errorf("no accessible clusters found in organization %q; check your API token permissions and cluster access", org)
	}

	var matches []buildkite.Cluster
	for _, c := range clusters {
		if c.Name == clusterName {
			matches = append(matches, c)
		}
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}

	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Name == clusters[j].Name {
			return clusters[i].ID < clusters[j].ID
		}
		return clusters[i].Name < clusters[j].Name
	})
	var message strings.Builder
	if len(matches) > 1 {
		fmt.Fprintf(&message, "multiple clusters named %q found in organization %q", clusterName, org)
	} else {
		fmt.Fprintf(&message, "no cluster named %q found in organization %q", clusterName, org)
	}
	message.WriteString("\n\nAvailable clusters:\n")
	for _, c := range clusters {
		fmt.Fprintf(&message, "  %s  %s\n", c.Name, c.ID)
	}
	if len(matches) > 1 {
		message.WriteString("\nRun again with --cluster-uuid <uuid> to select a cluster unambiguously.")
	} else {
		message.WriteString("\nRun again with --cluster-name <name> or --cluster-uuid <uuid> to select a cluster.")
	}
	return "", fmt.Errorf("%s", message.String())
}

// CreateAgentToken creates an agent token on the given cluster. The returned
// token's Token field holds the secret value, which the API only returns here.
func CreateAgentToken(ctx context.Context, f *factory.Factory, org, clusterID, description string) (buildkite.ClusterToken, error) {
	token, _, err := f.RestAPIClient.ClusterTokens.Create(ctx, org, clusterID, buildkite.ClusterTokenCreate{
		Description: description,
	})
	return token, err
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
