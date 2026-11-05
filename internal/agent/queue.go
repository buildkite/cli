package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

// clusterQueue retains hosted, which go-buildkite v5.29.0 does not expose.
// A missing value must not be mistaken for a self-hosted queue.
type clusterQueue struct {
	buildkite.ClusterQueue
	Hosted *bool `json:"hosted"`
}

// FindQueue resolves an exact queue key within a cluster, or the cluster's
// configured default queue when key is empty. Only self-hosted queues can be
// used by a local agent. Selection errors list the available self-hosted keys.
func FindQueue(ctx context.Context, f *factory.Factory, org, clusterID, key string) (string, error) {
	var defaultID string
	if key == "" {
		cluster, _, err := f.RestAPIClient.Clusters.Get(ctx, org, clusterID)
		if err != nil {
			return "", fmt.Errorf("getting cluster %q in organization %q: %w", clusterID, org, err)
		}
		defaultID = cluster.DefaultQueueID
	}

	var queues []clusterQueue
	for page := 1; ; {
		path := fmt.Sprintf("v2/organizations/%s/clusters/%s/queues?page=%d&per_page=100", url.PathEscape(org), url.PathEscape(clusterID), page)
		req, err := f.RestAPIClient.NewRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return "", fmt.Errorf("creating queue request: %w", err)
		}
		var batch []clusterQueue
		resp, err := f.RestAPIClient.Do(req, &batch)
		if err != nil {
			return "", fmt.Errorf("listing queues for cluster %q in organization %q: %w", clusterID, org, err)
		}
		queues = append(queues, batch...)
		if resp.NextPage == 0 {
			break
		}
		if resp.NextPage <= page {
			return "", fmt.Errorf("listing queues for cluster %q in organization %q: pagination did not advance from page %d", clusterID, org, page)
		}
		page = resp.NextPage
	}

	var selected *clusterQueue
	var available []string
	unknownHosting := false
	for idx := range queues {
		q := &queues[idx]
		if q.Hosted == nil {
			unknownHosting = true
		}
		if q.Hosted != nil && !*q.Hosted {
			available = append(available, q.Key)
		}
		if selected == nil && ((key != "" && q.Key == key) || (key == "" && defaultID != "" && q.ID == defaultID)) {
			selected = q
		}
	}
	if selected != nil && selected.Hosted != nil && !*selected.Hosted {
		return selected.Key, nil
	}

	var reason string
	switch {
	case selected != nil && selected.Hosted == nil:
		reason = fmt.Sprintf("cannot verify whether queue %q is self-hosted", selected.Key)
	case selected != nil:
		reason = fmt.Sprintf("queue %q is hosted; local agents require a self-hosted queue", selected.Key)
	case key != "":
		reason = fmt.Sprintf("no queue with key %q found", key)
	case defaultID == "":
		reason = "no default queue is configured"
	default:
		reason = fmt.Sprintf("configured default queue %q was not found", defaultID)
	}
	sort.Strings(available)
	var message strings.Builder
	fmt.Fprintf(&message, "%s in cluster %q (organization %q).\n\nAvailable self-hosted queue keys:", reason, clusterID, org)
	if len(available) == 0 && unknownHosting {
		message.WriteString("\n  (none verified)\n\nThe API omitted queue hosting information. Contact Buildkite support if this persists; local agents require a verified self-hosted queue before using --queue <key>.")
	} else if len(available) == 0 {
		message.WriteString("\n  (none)\n\nCreate a self-hosted queue in this cluster, then run again with --queue <key>.")
	} else {
		for _, k := range available {
			fmt.Fprintf(&message, "\n  %s", k)
		}
		message.WriteString("\n\nRun again with --queue <key> to select a self-hosted queue.")
	}
	return "", fmt.Errorf("%s", message.String())
}
