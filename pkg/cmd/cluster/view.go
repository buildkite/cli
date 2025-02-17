// pkg/cmd/cluster/view.go

package cluster

import (
	"bytes"
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
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

func NewCmdClusterView(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view <id>",
		Args:                  cobra.MinimumNArgs(1),
		Short:                 "View cluster information.",
		Long: heredoc.Doc(`
			View cluster information.

			It accepts org slug and cluster id.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			var cluster buildkite.Cluster
			spinErr := spinner.New().
				Title("Loading cluster information").
				Action(func() {
					cluster, _, err = f.RestAPIClient.Clusters.Get(cmd.Context(), f.Config.OrganizationSlug(), args[0])
				}).
				Run()
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

			return output.Write(cmd.OutOrStdout(), view, format)
		},
	}

	output.AddFlags(cmd.Flags())

	return &cmd
}
