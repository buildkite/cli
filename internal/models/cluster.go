package models

// Cluster represents a Buildkite cluster for agent organization
type Cluster struct {
	ID             string `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID      string `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	DefaultQueueID string `json:"default_queue_id,omitempty" yaml:"default_queue_id,omitempty"`
	Name           string `json:"name,omitempty" yaml:"name,omitempty"`
	Description    string `json:"description,omitempty" yaml:"description,omitempty"`
	Emoji          string `json:"emoji,omitempty" yaml:"emoji,omitempty"`
	Color          string `json:"color,omitempty" yaml:"color,omitempty"`
	URL            string `json:"url,omitempty" yaml:"url,omitempty"`
	WebURL         string `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	DefaultQueueURL string `json:"default_queue_url,omitempty" yaml:"default_queue_url,omitempty"`
	QueuesURL      string `json:"queues_url,omitempty" yaml:"queues_url,omitempty"`
	CreatedAt      string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	CreatedBy      *User  `json:"created_by,omitempty" yaml:"created_by,omitempty"`
	
	// Fields for internal CLI use
	Organization   *Organization `json:"-" yaml:"-"`
	Token          string        `json:"-" yaml:"-"`
}