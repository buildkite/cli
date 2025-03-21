package models

// Organization represents a Buildkite organization
type Organization struct {
	ID          string `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID   string `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
	WebURL      string `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Slug        string `json:"slug,omitempty" yaml:"slug,omitempty"`
	AgentsURL   string `json:"agents_url,omitempty" yaml:"agents_url,omitempty"`
	PipelinesURL string `json:"pipelines_url,omitempty" yaml:"pipelines_url,omitempty"`
	EmojisURL   string `json:"emojis_url,omitempty" yaml:"emojis_url,omitempty"`
	CreatedAt   string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}