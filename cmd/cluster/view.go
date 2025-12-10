package cluster

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
)

// CreatedByView represents cluster creator information
type CreatedByView struct {
	ID        string    `json:"id" yaml:"id"`
	GraphQLID string    `json:"graphql_id" yaml:"graphql_id"`
	Name      string    `json:"name" yaml:"name"`
	Email     string    `json:"email" yaml:"email"`
	AvatarURL string    `json:"avatar_url" yaml:"avatar_url"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// ClusterView provides a formatted view of cluster data
type ClusterView struct {
	ID              string         `json:"id" yaml:"id"`
	GraphQLID       string         `json:"graphql_id" yaml:"graphql_id"`
	DefaultQueueID  string         `json:"default_queue_id" yaml:"default_queue_id"`
	Name            string         `json:"name" yaml:"name"`
	Description     string         `json:"description,omitempty" yaml:"description,omitempty"`
	Emoji           string         `json:"emoji,omitempty" yaml:"emoji,omitempty"`
	Color           string         `json:"color,omitempty" yaml:"color,omitempty"`
	URL             string         `json:"url" yaml:"url"`
	WebURL          string         `json:"web_url" yaml:"web_url"`
	DefaultQueueURL string         `json:"default_queue_url" yaml:"default_queue_url"`
	QueuesURL       string         `json:"queues_url" yaml:"queues_url"`
	CreatedAt       time.Time      `json:"created_at" yaml:"created_at"`
	CreatedBy       *CreatedByView `json:"created_by,omitempty" yaml:"created_by,omitempty"`
}

// TextOutput implements the output.Formatter interface
func (c ClusterView) TextOutput() string {
	var b bytes.Buffer

	// Helper functions for consistent styling
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	label := lipgloss.NewStyle().Width(15).Bold(true)
	section := func(name string) {
		fmt.Fprintf(&b, "\n%s\n", title.Render(name))
	}
	field := func(name, value string) {
		fmt.Fprintf(&b, "%s %s\n", label.Render(name+":"), value)
	}

	// Basic Information
	section("Cluster Details")
	field("Name", c.Name)
	if c.Emoji != "" {
		field("Emoji", c.Emoji)
	}
	if c.Description != "" {
		field("Description", c.Description)
	}
	if c.Color != "" {
		field("Color", c.Color)
	}

	// IDs and URLs
	section("Identifiers")
	field("ID", c.ID)
	field("GraphQL ID", c.GraphQLID)
	field("Default Queue ID", c.DefaultQueueID)

	// URLs
	section("URLs")
	field("Web URL", c.WebURL)
	field("API URL", c.URL)
	field("Queues URL", c.QueuesURL)
	field("Queue URL", c.DefaultQueueURL)

	// Creator Information
	if c.CreatedBy != nil {
		section("Created By")
		field("Name", c.CreatedBy.Name)
		field("Email", c.CreatedBy.Email)
		field("ID", c.CreatedBy.ID)
		field("Created At", c.CreatedAt.Format(time.RFC3339))
	}

	return b.String()
}

type ViewCmd struct {
	ClusterID string `arg:"" help:"Cluster ID to view"`
	Output    string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}"`
}

func (c *ViewCmd) Help() string {
	return `
It accepts cluster id.

Examples:
  # View a cluster
  $ bk cluster view my-cluster-id

  # View cluster in JSON format
  $ bk cluster view my-cluster-id -o json
`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	format := output.Format(c.Output)
	if format != output.FormatJSON && format != output.FormatYAML && format != output.FormatText {
		return fmt.Errorf("invalid output format: %s", c.Output)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var cluster buildkite.Cluster
	spinErr := bkIO.SpinWhile(f, "Loading cluster information", func() {
		cluster, _, err = f.RestAPIClient.Clusters.Get(ctx, f.Config.OrganizationSlug(), c.ClusterID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	view := ClusterView{
		ID:              cluster.ID,
		GraphQLID:       cluster.GraphQLID,
		DefaultQueueID:  cluster.DefaultQueueID,
		Name:            cluster.Name,
		Description:     cluster.Description,
		Emoji:           cluster.Emoji,
		Color:           cluster.Color,
		URL:             cluster.URL,
		WebURL:          cluster.WebURL,
		DefaultQueueURL: cluster.DefaultQueueURL,
		QueuesURL:       cluster.QueuesURL,
		CreatedAt:       cluster.CreatedAt.Time,
	}

	if cluster.CreatedBy.ID != "" {
		view.CreatedBy = &CreatedByView{
			ID:        cluster.CreatedBy.ID,
			GraphQLID: cluster.CreatedBy.GraphQLID,
			Name:      cluster.CreatedBy.Name,
			Email:     cluster.CreatedBy.Email,
			AvatarURL: cluster.CreatedBy.AvatarURL,
			CreatedAt: cluster.CreatedBy.CreatedAt.Time,
		}
	}

	return output.Write(os.Stdout, view, format)
}
