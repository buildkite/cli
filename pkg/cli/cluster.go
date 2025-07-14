package cli

import (
	"context"
	"fmt"

	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// Cluster commands
type ClusterCmd struct {
	List ClusterListCmd `cmd:"" help:"List clusters"`
	View ClusterViewCmd `cmd:"" help:"View cluster details"`
}

type ClusterListCmd struct {
	OutputFlag   `embed:""`
	Organization string `help:"Organization slug (if omitted, uses configured organization)" placeholder:"my-org"`
}

type ClusterViewCmd struct {
	OutputFlag `embed:""`
	Cluster    string `arg:"" help:"Cluster UUID to view"`
}

// Cluster command implementations
func (c *ClusterListCmd) Run(ctx context.Context, f *factory.Factory) error {
	c.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	org := c.Organization
	if org == "" {
		org = f.Config.OrganizationSlug()
	}

	// List clusters
	var err error
	var clusters []buildkite.Cluster
	spinErr := bk_io.SpinWhile("Loading clusters", func() {
		clusters, _, err = f.RestAPIClient.Clusters.List(ctx, org, nil)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error listing clusters: %w", err)
	}

	if len(clusters) == 0 {
		if ShouldUseStructuredOutput(f) {
			return Print([]any{}, f)
		}
		fmt.Println("No clusters found")
		return nil
	}

	// Output clusters in requested format
	if ShouldUseStructuredOutput(f) {
		return Print(clusters, f)
	}

	// Display clusters
	for _, cluster := range clusters {
		fmt.Printf("ID: %s\n", cluster.ID)
		fmt.Printf("Name: %s\n", cluster.Name)
		if cluster.Description != "" {
			fmt.Printf("Description: %s\n", cluster.Description)
		}
		fmt.Printf("URL: %s\n", cluster.WebURL)
		fmt.Println("---")
	}

	return nil
}

func (c *ClusterViewCmd) Run(ctx context.Context, f *factory.Factory) error {
	c.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	org := f.Config.OrganizationSlug()

	// Get cluster details
	var err error
	var cluster buildkite.Cluster
	spinErr := bk_io.SpinWhile("Loading cluster", func() {
		cluster, _, err = f.RestAPIClient.Clusters.Get(ctx, org, c.Cluster)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error getting cluster: %w", err)
	}

	// Output cluster information in requested format
	if ShouldUseStructuredOutput(f) {
		return Print(cluster, f)
	}

	// Display cluster information
	fmt.Printf("Cluster: %s\n", cluster.Name)
	fmt.Printf("ID: %s\n", cluster.ID)
	if cluster.Description != "" {
		fmt.Printf("Description: %s\n", cluster.Description)
	}
	fmt.Printf("URL: %s\n", cluster.WebURL)
	fmt.Printf("Created: %s\n", cluster.CreatedAt)

	return nil
}
